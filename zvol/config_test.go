package zvol

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestNewConfigFromToml(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		want := Config{
			RootPath:   "/tmp",
			Dataset:    "tank/snapshots",
			VolumeSize: "50G",
		}

		file, err := os.CreateTemp("", "zvol-snapshotter-config-")
		if err != nil {
			t.Error(err)
		}

		encoder := toml.NewEncoder(file)
		if err := encoder.Encode(&want); err != nil {
			t.Error(err)
		}

		defer func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}

			if err := os.Remove(file.Name()); err != nil {
				t.Error(err)
			}
		}()

		got, err := NewConfigFromToml(file.Name())
		if err != nil {
			t.Errorf("want nil, got error: %s", err)
		}

		if got.RootPath != want.RootPath {
			t.Errorf("want config.RootPath: %s, got: %s", want.RootPath, got.RootPath)
		}

		if got.Dataset != want.Dataset {
			t.Errorf("want config.Dataset: %s, got: %s", want.Dataset, got.Dataset)
		}

		if got.volumeSizeBytes != 50*1024*1024*1024 {
			t.Errorf("want config.volumeSizeBytes: %d, got: %d", 50*1024*1024*1024, got.volumeSizeBytes)
		}

		if got.FileSystemType != fsTypeExt4 {
			t.Errorf("want config.FileSystemType: %s, got: %s", fsTypeExt4, got.FileSystemType)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		_, err := NewConfigFromToml("")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("want: %s, got: %s", fs.ErrNotExist, err)
		}
	})

	t.Run("invalid data", func(t *testing.T) {
		cfg := Config{
			VolumeSize: "x",
		}

		file, err := os.CreateTemp("", "zvol-snapshotter-config-")
		if err != nil {
			t.Error(err)
		}

		encoder := toml.NewEncoder(file)
		if err := encoder.Encode(&cfg); err != nil {
			t.Error(err)
		}

		defer func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}

			if err := os.Remove(file.Name()); err != nil {
				t.Error(err)
			}
		}()

		_, err = NewConfigFromToml(file.Name())
		if err == nil {
			t.Errorf("want error, got nil")
		}
	})
}

func TestConfigFieldValidation(t *testing.T) {
	t.Run("invalid field validation", func(t *testing.T) {
		cfg := Config{}
		err := cfg.Validate()

		multErr := err.(interface{ Unwrap() []error }).Unwrap()
		if len(multErr) != 3 {
			t.Errorf("want %d errors, got %d", 3, len(multErr))
		}
	})

	t.Run("valid field validation", func(t *testing.T) {
		cfg := Config{
			RootPath:       "/tmp",
			Dataset:        "tank/snapshots",
			FileSystemType: "ext4",
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("want nil, get error: %s", err)
		}
	})
}
