## ZVOL Snapshotter

> 🛠 **Status: experimental**
>
> This project is a work in progress.

ZFS Volume snapshotter plugin for [containerd](https://github.com/containerd/containerd).


## Getting started

Zvol snapshotter is implemented as a proxy plugin daemon. You need the following containerd configuration and run the zvol snapshotter daemon.

- Update containerd config file which by default is located at `/etc/containerd/config.toml`.

    ```toml
    # Plug vvol snapshotter into containerd
    # Containerd recognizes the zvol snapshotter through specified socket address.
    # The specified address below is the default which the zvol snapshotter listens to.
    [proxy_plugins]
      [proxy_plugins.zvol]
        type = "snapshot"
        address = "/run/containerd-zvol.sock"
    ```
- Run  zvol snapshotter daemon.

    ```sh
    sudo go run cmd/main.go -dataset zroot/snapshots
    ```

## License

Zvol Snapshotter (c) 2025 Han Verstraete

SPDX-License-Identifier: Apache-2.0 
