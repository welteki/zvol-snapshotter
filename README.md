## Zvol Snapshotter

ZFS Volume snapshotter plugin for [containerd](https://github.com/containerd/containerd).

## Install

Zvol snapshotter is implemented as a proxy plugin daemon. To use Zvol snapshotter you will need to create a zfs pool or dataset to be used for snapshots, run the Zvol snapshotter daemon and register the snapshotter plugin with containerd.

### Requirements

- [containerd](https://github.com/containerd/containerd/blob/main/docs/getting-started.md) >= 1.4
- ZFS - On ubuntu it can be installed with `sudo apt install zfsutils-linux`

### Create a ZFS dataset

Create a ZFS dataset that will be used for snapshots. The dataset name is arbitrary, pick whatever you want.

```sh
sudo zfs create your-zpool/snapshots
```

### Run snapshotter daemon

You can download prebuilt binaries for the snapshotter from the [release page](https://github.com/welteki/zvol-snapshotter/releases) or [build them from source](#build-zvol-snapshotter-from-source).

```sh
version="0.2.1"
arch="amd64"

wget https://github.com/welteki/zvol-snapshotter/releases/download/v${version}/zvol-snapshotter-${version}-linux-${arch}.tar.gz
sudo tar -C /usr/local/bin \
  -xvf zvol-snapshotter-${version}-linux-${arch}.tar.gz containerd-zvol-grpc
```

**Run snapshotter**

```sh
sudo containerd-zvol-grpc -dataset=your-zpool/snapshots
```

**Run snapshotter as a systemd service**

To run the Zvol snapshotter process as a systemd service you can download the [zvol-snaphsotter.service unit file](https://github.com/welteki/zvol-snapshotter/blob/main/scripts/config/zvol-snapshotter.service) into `/etc/systemd/system/zvol-snapshotter.service`.

After saving the service file, you can start the service with the usual systemctl dance:

```sh
sudo systemctl daemon-reload
sudo systemctl enable zvol-snapshotter
sudo systemctl start zvol-snapshotter
```

### Configure containerd

Configure and restart containerd to enable Zvol snapshotter. (this section assumes your containerd is managed by systemd)

- Update containerd config file which by default is located at `/etc/containerd/config.toml`.

    ```toml
    # Plug zvol snapshotter into containerd
    # Containerd recognizes zvol snapshotter through the specified socket address.
    # The specified address below is the default which zvol snapshotter listens to.
    [proxy_plugins]
      [proxy_plugins.zvol]
        type = "snapshot"
        address = "/run/containerd-zvol-grpc/containerd-zvol-grpc.sock"
    ```
- Restart containerd: `sudo systemctl restart containerd`
- Check to make sure Zvol snapshotter is recognized by containerd: `sudo ctr plugin ls id==zvol`

### Run

Try out the snapshotter with the following command:

```sh
ctr images pull --snapshotter zvol docker.io/library/hello-world:latest
ctr run --snapshotter zvol docker.io/library/hello-world:latest test
```

## Configuration

The Zvol snapshotter has a toml config file that is located at `/etc/containerd-zvol-grpc/config.toml` by default. If such a file does not exist, Zvol snapshotter will use default values for all configurations.

Example configuration:

```toml
root_path="/var/lib/containerd-zvol-grpc"
dataset="your-zpool/snapshots"
volume_size="20G"
fs_type="ext4"
```

The following configuration settings are available:

- `root_path` - Snapshotter root directory for metadata.
- `dataset` - ZFS dataset that will be used for snapshots.
- `volume_size` - Space to allocate when creating volumes.
- `fs_type` - File system to use for snapshot device mounts. (Currently only ext4 is supported)

## Label Propagation to ZFS

Containerd snapshot labels are automatically stored as ZFS user properties on the underlying datasets. This makes it possible to identify and query ZFS volumes and snapshots based on container metadata using standard `zfs` commands.

Labels are stored with the prefix `containerd:label.` and the label name is sanitized to comply with ZFS property naming rules: characters are lowercased, `/` becomes `_`, while `.`, `:`, `+`, and `_` are preserved.

When a snapshot is committed, the labels are written to the ZFS volume before the ZFS snapshot is taken, so the `@snapshot` automatically inherits them. When cloning from a parent snapshot, only the labels provided by containerd for the new snapshot are set on the clone.

### Querying ZFS datasets by label

List all datasets with any containerd label:

```sh
zfs get all -r your-zpool/snapshots -o name,property,value | grep "containerd:label"
```

Find datasets by a custom application label (e.g. `myapp/environment`):

```sh
zfs get containerd:label.myapp_environment \
  -r your-zpool/snapshots -o name,value -Hp | grep -v $'\t-'
```

## Build Zvol snapshotter from source

Checkout the source code using git clone:

```sh
git clone https://github.com/weltei/zvol-snapshotter.git
cd zvol-snapshotter
```

`make` is used as the build tool. Assuming you are in the root directory build the snapshotter by running:

```sh
make
```

The snapshotter binary is build into the `./out` directory. Install to a `PATH` directory with:

```sh
sudo make install
# check to make sure the Zvol snapshotter can be found in PATH
sudo containerd-zvol-grpc -version
```

The binary is installed in `/usr/local/bin` by default. Set `CMD_DESTDIR` to change the destination.

## License

Zvol Snapshotter (c) 2025 Han Verstraete

SPDX-License-Identifier: Apache-2.0
