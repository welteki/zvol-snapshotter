package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/log"
	"github.com/welteki/zvol-snapshotter/version"
	"github.com/welteki/zvol-snapshotter/zvol"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

const (
	defaultAddress  = "/run/containerd-zvol.sock"
	defaultLogLevel = log.InfoLevel
	defaultRootDir  = "/var/lib/containerd-zvol"
)

var (
	address = flag.String("address", defaultAddress, "address for the snapshotter's GRPC server")
	// configPath        = flag.String("config", defaultConfigPath, "path to the configuration file")
	logLevel     = flag.String("log-level", defaultLogLevel.String(), "set the logging level [trace, debug, info, warn, error, fatal, panic]")
	rootDir      = flag.String("root", defaultRootDir, "path to the root directory for this snapshotter")
	dataset      = flag.String("dataset", "", "zfs dataset used for snapshots")
	printVersion = flag.Bool("version", false, "print the version")
)

func main() {
	flag.Parse()

	err := log.SetLevel(*logLevel)
	if err != nil {
		log.L.WithError(err).Fatal("failed to prepare logger")
	}

	if *printVersion {
		fmt.Println("containerd-zvol-snapshotter", version.Version, version.Revision)
		return
	}

	ctx := context.Background()
	snapshotterConfig := zvol.Config{
		RootPath: *rootDir,
		Dataset:  *dataset,
	}

	if snapshotterConfig.Dataset == "" {
		log.G(ctx).WithError(err).Fatalf("-dataset is required")
	}

	// Create a gRPC server
	rpc := grpc.NewServer()

	// Create snapshotter
	sn, err := zvol.NewSnapshotter(ctx, &snapshotterConfig)
	if err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to create snapshotter")
	}

	if err := serve(ctx, rpc, *address, sn); err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to serve snapshotter")
	}

	log.G(ctx).Info("Exiting")
}

func serve(ctx context.Context, rpc *grpc.Server, addr string, sn snapshots.Snapshotter) error {
	// Convert the snapshotter to a gRPC service,
	service := snapshotservice.FromSnapshotter(sn)

	// Register the service with the gRPC server
	snapshotsapi.RegisterSnapshotsServer(rpc, service)

	// Prepare the directory for the socket
	if err := os.MkdirAll(filepath.Dir(addr), 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(addr), err)
	}

	// Try to remove the socket file to avoid EADDRINUSE
	if err := os.RemoveAll(addr); err != nil {
		return fmt.Errorf("failed to remove %q: %w", addr, err)
	}

	// Listen and serve
	l, err := net.Listen("unix", addr)
	if err != nil {
		return fmt.Errorf("error listening on socket %q: %w", addr, err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := rpc.Serve(l); err != nil {
			errChan <- fmt.Errorf("error serving on socket %q: %w", addr, err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, unix.SIGINT, unix.SIGTERM)

	select {
	case sig := <-sigChan:
		log.G(ctx).Infof("Received signal %v", sig)
		if sig == unix.SIGINT {
			log.G(ctx).Debug("Closing the snapshotter")
			sn.Close()
		}
		return nil
	case err := <-errChan:
		return err
	}
}
