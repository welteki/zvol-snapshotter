package zvol

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/log"
	"github.com/mistifyio/go-zfs/v3"
)

type fsType string

const fsTypeExt4 fsType = "ext4"

const (
	// LabelVolumeSize is the label used for the volume size
	LabelVolumeSize = "containerd.io/snapshot/zvol/size"

	zfsDevicePath = "/dev/zvol"

	// snapshotSuffix is used as follows:
	//	active := filepath.Join(dataset.Name, id)
	//      committed := active + "@" + snapshotSuffix
	snapshotSuffix = "snapshot"

	// Default volume size to use if not specified in configs
	defaultVolumeSize uint64 = 20 * 1024 * 1024 * 1024 //20G

	maxSnapshotSize int64 = math.MaxInt64
)

type snapshotter struct {
	dataset *zfs.Dataset
	store   *storage.MetaStore
	config  *Config
}

func NewSnapshotter(ctx context.Context, config *Config) (snapshots.Snapshotter, error) {
	if err := config.parse(); err != nil {
		return nil, err
	}

	dataset, err := zfs.GetDataset(config.Dataset)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(config.RootPath, 0750); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("failed to create root directory: %s: %w", config.RootPath, err)
	}

	ms, err := storage.NewMetaStore(filepath.Join(config.RootPath, "metadata.db"))
	if err != nil {
		return nil, err
	}

	z := &snapshotter{
		dataset: dataset,
		store:   ms,
		config:  config,
	}

	return z, nil
}

var zfsCreateVolumeProperties = map[string]string{
	"refreservation": "none",
	"volmode":        "full",
}

// Stat returns the info for an active or committed snapshot by name or
// key.
//
// Should be used for parent resolution, existence checks and to discern
// the kind of snapshot.
func (s *snapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	log.G(ctx).WithField("key", key).Debug("stat")

	var (
		info snapshots.Info
		err  error
	)

	err = s.store.WithTransaction(ctx, false, func(ctx context.Context) error {
		_, info, _, err = storage.GetInfo(ctx, key)
		return err
	})

	return info, err
}

// Update updates the info for a snapshot.
//
// Only mutable properties of a snapshot may be updated.
func (s *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	log.G(ctx).Debugf("update: %s", strings.Join(fieldpaths, ", "))

	var err error
	err = s.store.WithTransaction(ctx, true, func(ctx context.Context) error {
		info, err = storage.UpdateInfo(ctx, info, fieldpaths...)
		return err
	})

	return info, err
}

// Usage returns the resource usage of an active or committed snapshot
// excluding the usage of parent snapshots.
//
// The running time of this call for active snapshots is dependent on
// implementation, but may be proportional to the size of the resource.
// Callers should take this into consideration. Implementations should
// attempt to honor context cancellation and avoid taking locks when making
// the calculation.
func (s *snapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	log.G(ctx).WithField("key", key).Debug("usage")

	var (
		usage snapshots.Usage
		err   error
	)

	err = s.store.WithTransaction(ctx, false, func(ctx context.Context) error {
		usage, err = s.usage(ctx, key)
		return err
	})

	return usage, err
}

func (s *snapshotter) usage(ctx context.Context, key string) (snapshots.Usage, error) {
	id, info, usage, err := storage.GetInfo(ctx, key)
	if err != nil {
		return snapshots.Usage{}, err
	}

	if info.Kind == snapshots.KindActive {
		activeName := filepath.Join(s.dataset.Name, id)
		sDataset, err := zfs.GetDataset(activeName)
		if err != nil {
			return snapshots.Usage{}, err
		}

		if int64(sDataset.Used) > maxSnapshotSize {
			return snapshots.Usage{}, fmt.Errorf("dataset size exceeds maximum snapshot size of %d bytes", maxSnapshotSize)
		}

		usage = snapshots.Usage{
			Size:   int64(sDataset.Used),
			Inodes: -1,
		}
	}

	return usage, nil
}

// Mounts returns the mounts for the active snapshot transaction identified
// by key. Can be called on a read-write or readonly transaction. This is
// available only for active snapshots.
//
// This can be used to recover mounts after calling View or Prepare.
func (s *snapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	log.G(ctx).WithField("key", key).Debug("mounts")

	var (
		snap storage.Snapshot
		err  error
	)

	err = s.store.WithTransaction(ctx, false, func(ctx context.Context) error {
		snap, err = storage.GetSnapshot(ctx, key)
		return err
	})
	if err != nil {
		return nil, err
	}

	snapName := filepath.Join(s.dataset.Name, snap.ID)
	snapDataset, err := zfs.GetDataset(snapName)
	if err != nil {
		return nil, err
	}
	return getMounts(snapDataset, false), nil
}

// Prepare creates an active snapshot identified by key descending from the
// provided parent.  The returned mounts can be used to mount the snapshot
// to capture changes.
//
// If a parent is provided, after performing the mounts, the destination
// will start with the content of the parent. The parent must be a
// committed snapshot. Changes to the mounted destination will be captured
// in relation to the parent. The default parent, "", is an empty
// directory.
//
// The changes may be saved to a committed snapshot by calling Commit. When
// one is done with the transaction, Remove should be called on the key.
//
// Multiple calls to Prepare or View with the same key should fail.
func (s *snapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.G(ctx).WithFields(log.Fields{"key": key, "parent": parent}).Debug("prepare")

	var (
		mounts []mount.Mount
		err    error
	)

	err = s.store.WithTransaction(ctx, true, func(ctx context.Context) error {
		mounts, err = s.createSnapshot(ctx, snapshots.KindActive, key, parent, opts...)
		return err
	})

	return mounts, err
}

// View behaves identically to Prepare except the result may not be
// committed back to the snapshot snapshotter. View returns a readonly view on
// the parent, with the active snapshot being tracked by the given key.
//
// This method operates identically to Prepare, except the mounts returned
// may have the readonly flag set. Any modifications to the underlying
// filesystem will be ignored. Implementations may perform this in a more
// efficient manner that differs from what would be attempted with
// `Prepare`.
//
// Commit may not be called on the provided key and will return an error.
// To collect the resources associated with key, Remove must be called with
// key as the argument.
func (s *snapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.G(ctx).WithFields(log.Fields{"key": key, "parent": parent}).Debug("view")

	var (
		mounts []mount.Mount
		err    error
	)

	err = s.store.WithTransaction(ctx, true, func(ctx context.Context) error {
		mounts, err = s.createSnapshot(ctx, snapshots.KindView, key, parent, opts...)
		return err
	})

	return mounts, err
}

func (s *snapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	volSize := s.config.volumeSizeBytes
	if len(parent) > 0 {
		_, snapInfo, _, err := storage.GetInfo(ctx, parent)
		if err != nil {
			log.G(ctx).Errorf("failed to read snapshotInfo for %s", parent)
			return nil, err
		}

		if v, ok := snapInfo.Labels[LabelVolumeSize]; ok {
			volSize, err = strconv.ParseUint(v, 10, 64)
			if err != nil {
				log.G(ctx).Errorf("failed to parse volume size for %s", parent)
				return nil, err
			}
		}
	}

	labels := getLabelOpts(opts...)
	if v, ok := labels[LabelVolumeSize]; ok {
		val, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			log.G(ctx).Errorf("failed to parse volume size for %s", key)
			return nil, err
		}

		if val < volSize {
			return nil, fmt.Errorf("invalid volume size for snapshot %s, must be greater than or equal to parent volume size: %d", key, volSize)
		}

		volSize = val
	}

	opts = append(opts, WithVolumeSize(volSize))

	snap, err := storage.CreateSnapshot(ctx, kind, key, parent, opts...)
	if err != nil {
		return nil, err
	}

	targetName := filepath.Join(s.dataset.Name, snap.ID)
	var target *zfs.Dataset
	if len(snap.ParentIDs) == 0 {
		log.G(ctx).Debugf("creating new zfs volume '%s'", targetName)

		target, err = zfs.CreateVolume(targetName, volSize, zfsCreateVolumeProperties)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to create zfs volume for snapshot %s", snap.ID)
			return nil, err
		}
		devicePath := getDevicePath(target)

		// Wait for Zvol symlinks to be created under /dev/zvol.
		waitForFile(ctx, devicePath)

		// ext4 options taken from device mapper.
		// Explicitly disable lazy_itable_init and lazy_journal_init in order to enable lazy initialization.
		fsOptions := "nodiscard,lazy_itable_init=0,lazy_journal_init=0"
		log.G(ctx).Debugf("creating file system of type: %s with options: %s on zfs volume %q", fsTypeExt4, fsOptions, target.Name)
		if err := mkfs(ctx, fsTypeExt4, fsOptions, devicePath); err != nil {
			errs := []error{err}

			// Rollback zfs volume creation if mkfs failed
			errs = append(errs, target.Destroy(zfs.DestroyDefault))

			log.G(ctx).WithError(errors.Join(errs...)).Errorf("failed to initialize zfs volume %q for snapshot %s", target.Name, snap.ID)
			return nil, errors.Join(errs...)
		}

		readonly := false
		mounts := getMounts(target, readonly)

		// Remove default directories not expected by the container image
		_ = mount.WithTempMount(ctx, mounts, func(root string) error {
			return os.Remove(filepath.Join(root, "lost+found"))
		})
	} else {
		parent0Name := filepath.Join(s.dataset.Name, snap.ParentIDs[0]+"@"+snapshotSuffix)
		parent0, err := zfs.GetDataset(parent0Name)
		if err != nil {
			return nil, err
		}
		target, err = parent0.Clone(targetName, zfsCreateVolumeProperties)
		if err != nil {
			return nil, err
		}

		// Resize target if required
		if volSize > 0 && parent0.Volsize != volSize {
			if err := target.SetProperty("volsize", fmt.Sprintf("%d", volSize)); err != nil {
				return nil, err
			}
		}

		// Wait for Zvol symlinks to be created under /dev/zvol.
		devicePath := getDevicePath(target)
		waitForFile(ctx, devicePath)
	}

	readonly := kind == snapshots.KindView
	return getMounts(target, readonly), nil
}

func getMounts(dataset *zfs.Dataset, readonly bool) []mount.Mount {
	var options []string
	if readonly {
		options = append(options, "ro")
	}
	return []mount.Mount{
		{
			Type:    fmt.Sprint(fsTypeExt4), // TODO: get fs type from dataset attributes
			Source:  getDevicePath(dataset),
			Options: options,
		},
	}
}

// Commit captures the changes between key and its parent into a snapshot
// identified by name.  The name can then be used with the snapshotter's other
// methods to create subsequent snapshots.
//
// A committed snapshot will be created under name with the parent of the
// active snapshot.
//
// After commit, the snapshot identified by key is removed.
func (s *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	log.G(ctx).WithFields(log.Fields{"name": name, "key": key}).Debug("commit")

	return s.store.WithTransaction(ctx, true, func(ctx context.Context) error {
		_, snapInfo, _, err := storage.GetInfo(ctx, key)
		if err != nil {
			return err
		}

		usage, err := s.usage(ctx, key)
		if err != nil {
			return err
		}

		volSizeLabel := snapInfo.Labels[LabelVolumeSize]
		if volSizeLabel != "" {
			labels := make(map[string]string)
			labels[LabelVolumeSize] = volSizeLabel
			opts = append(opts, snapshots.WithLabels(labels))
		}

		id, err := storage.CommitActive(ctx, key, name, usage, opts...)
		if err != nil {
			return err
		}

		activeName := filepath.Join(s.dataset.Name, id)
		active, err := zfs.GetDataset(activeName)
		if err != nil {
			return err
		}

		if _, err := active.Snapshot(snapshotSuffix, false); err != nil {
			return err
		}

		// After committing the snapshot volume will not be directly
		// used anymore. Setting volmode to none ensures the volume is not exposed outside of ZFS.
		// It can still be snapshotted and cloned.
		if err := active.SetProperty("volmode", "none"); err != nil {
			return err
		}

		return nil
	})
}

// Remove the committed or active snapshot by the provided key.
//
// All resources associated with the key will be removed.
//
// If the snapshot is a parent of another snapshot, its children must be
// removed before proceeding.
func (s *snapshotter) Remove(ctx context.Context, key string) error {
	log.G(ctx).WithField("key", key).Debug("remove")

	return s.store.WithTransaction(ctx, true, func(ctx context.Context) error {
		id, k, err := storage.Remove(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to remove snapshot: %w", err)
		}

		datasetName := filepath.Join(s.dataset.Name, id)
		if k == snapshots.KindCommitted {
			snapshotName := datasetName + "@" + snapshotSuffix
			snapshot, err := zfs.GetDataset(snapshotName)
			if err != nil {
				return err
			}
			if err = snapshot.Destroy(zfs.DestroyDeferDeletion); err != nil {
				return err
			}
		}
		dataset, err := zfs.GetDataset(datasetName)
		if err != nil {
			return err
		}
		if err = dataset.Destroy(zfs.DestroyDefault); err != nil {
			return err
		}

		return err
	})
}

// Walk will call the provided function for each snapshot in the
// snapshotter which match the provided filters. If no filters are
// given all items will be walked.
// Filters:
//
//	name
//	parent
//	kind (active,view,committed)
//	labels.(label)
func (s *snapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	log.G(ctx).Debug("walk")

	return s.store.WithTransaction(ctx, false, func(ctx context.Context) error {
		return storage.WalkInfo(ctx, fn, filters...)
	})
}

// Close releases the internal resources.
//
// Close is expected to be called on the end of the lifecycle of the snapshotter,
// but not mandatory.
//
// Close returns nil when it is already closed.
func (s *snapshotter) Close() error {
	log.L.Debug("close")

	return s.store.Close()
}

func getDevicePath(dataset *zfs.Dataset) string {
	return path.Join(zfsDevicePath, dataset.Name)
}

// mkfs creates a filesystem on the given zfs volume
func mkfs(ctx context.Context, fs fsType, fsOptions string, path string) error {
	if fs != fsTypeExt4 {
		return errors.New("file system not supported")
	}

	command := "mkfs.ext4"
	args := []string{
		"-E",
		fsOptions,
		path,
	}

	log.G(ctx).Debugf("%s %s", command, strings.Join(args, " "))
	o, err := exec.Command(command, args...).CombinedOutput()
	out := string(o)
	if err != nil {
		return fmt.Errorf("%s failed to initialize %q: %s: %w", command, path, out, err)
	}

	log.G(ctx).Debugf("mkfs:\n%s", out)
	return nil
}

func waitForFile(ctx context.Context, filePath string) {
	if _, err := os.Stat(filePath); err == nil {
		return
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(filePath); err == nil {
				return
			}
		}
	}
}

// WithVolumeSize sets the ZFS volume size for the created snapshot.
func WithVolumeSize(size uint64) snapshots.Opt {
	return func(info *snapshots.Info) error {
		if info.Labels == nil {
			info.Labels = make(map[string]string)
		}

		info.Labels[LabelVolumeSize] = fmt.Sprintf("%d", size)
		return nil
	}
}

func getLabelOpts(opts ...snapshots.Opt) map[string]string {
	info := &snapshots.Info{}
	for _, opt := range opts {
		opt(info)
	}
	return info.Labels
}
