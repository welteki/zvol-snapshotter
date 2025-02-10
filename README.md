## Zvol Snapshotter

> 🛠 **Status: experimental**
>
> This project is a work in progress.

ZFS Volume snapshotter plugin for [containerd](https://github.com/containerd/containerd).

## Getting started

Zvol snapshotter is implemented as a proxy plugin daemon. You will need to create a zfs pool or dataset to be used for snapshots, run the zvol snapshotter daemon and update the containerd configuration.

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

- Create ZFS dataset.

    The dataset name is arbitrary, pick whatever you want.

    ```sh
    sudo zfs create your-zpool/snapshots 
    ```
- Run  zvol snapshotter daemon.

    ```sh
    containerd-zvol-grpc -dataset your-zpool/snapshots
    ```

## License

Zvol Snapshotter (c) 2025 Han Verstraete

SPDX-License-Identifier: Apache-2.0 
