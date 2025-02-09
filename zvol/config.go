package zvol

import (
	"errors"
	"fmt"
	"os"

	"github.com/docker/go-units"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	// ZFS volume snapshotter root directory for metadata
	RootPath string `toml:"root_path"`

	// ZFS dataset that will be used for snapshots
	Dataset string `toml:"dataset"`

	// Defines how much space to allocate when creating volumes
	VolumeSize      string `toml:"volume_size"`
	volumeSizeBytes uint64 `toml:"-"`

	// Defines the file system to use for snapshot device mounts. Defaults to "ext4"
	FileSystemType fsType `toml:"fs_type"`
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

func (c *Config) Validate() error {
	var result []error

	if c.RootPath == "" {
		result = append(result, fmt.Errorf("root_path is required"))
	}

	if c.Dataset == "" {
		result = append(result, fmt.Errorf("dataset is required"))
	}

	if c.FileSystemType != "" {
		switch c.FileSystemType {
		case fsTypeExt4:
		default:
			result = append(result, fmt.Errorf("unsupported filesystem type: %q", c.FileSystemType))
		}
	} else {
		result = append(result, fmt.Errorf("fs_type is required"))
	}

	return errors.Join(result...)
}

func NewConfig() (*Config, error) {
	cfg := Config{}

	if err := cfg.parse(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func NewConfigFromToml(path string) (*Config, error) {
	configFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	config := Config{}
	if err := toml.NewDecoder(configFile).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config TOML: %w", err)
	}

	fmt.Println("no decode error")

	if err := config.parse(); err != nil {
		return nil, err
	}

	return &config, err
}
