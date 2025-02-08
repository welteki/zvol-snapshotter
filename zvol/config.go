package zvol

import (
	"fmt"

	"github.com/docker/go-units"
)

type Config struct {
	// ZFS volume snapshotter root directory for metadata
	RootPath string

	// ZFS dataset that will be used for snapshots
	Dataset string

	// Defines how much space to allocate when creating volumes
	VolumeSize      string
	volumeSizeBytes uint64

	// Defines the file system to use for snapshot device mounts. Defaults to "ext4"
	FileSystemType fsType
}

func (c *Config) parse() error {
	if c.VolumeSize == "" {
		c.volumeSizeBytes = defaultVolumeSize
	} else {
		volumeSize, err := units.RAMInBytes(c.VolumeSize)
		if err != nil {
			return fmt.Errorf("failed to parse volume size: '%s': %w", c.VolumeSize, err)
		}
		c.volumeSizeBytes = uint64(volumeSize)
	}

	if c.FileSystemType == "" {
		c.FileSystemType = fsTypeExt4
	}

	return nil
}
