#!/bin/bash

set -eux -o pipefail

CUR_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ZVOL_SNAPSHOTTER_PROJECT_ROOT="$(cd -- "$CUR_DIR"/.. && pwd)"
RELEASE_DIR="${RELEASE_DIR:-${ZVOL_SNAPSHOTTER_PROJECT_ROOT}/release}"
OUT_DIR="${ZVOL_SNAPSHOTTER_PROJECT_ROOT}/out"
TAG_REGEX="v[0-9]+.[0-9]+.[0-9]+"

if [ -z "$ARCH" ]; then
    case $(uname -m) in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *) echo "Error: Unsupported arch"; exit 1 ;;
    esac
fi

if [ "$ARCH" != "amd64" ] && [ "$ARCH" != "arm64" ]; then
    echo "Error: Unsupported arch ($ARCH)"
    exit 1
fi

if [ "$#" -ne 1 ]; then
    echo "Expected 1 parameter, got $#."
    echo "Usage: $0 [release_tag]"
    exit 1
fi

if ! [[ "$1" =~ $TAG_REGEX ]]; then
    echo "Improper tag format. Format should match regex $TAG_REGEX"
    exit 1
fi

if [ -d "$RELEASE_DIR" ]; then
    rm -rf "${RELEASE_DIR:?}"/*
else
    mkdir "$RELEASE_DIR"
fi

release_version=${1/v/} # Remove v from tag name
binary_name=zvol-snapshotter-${release_version}-linux-${ARCH}.tar.gz

GOARCH=${ARCH} make build
pushd "$OUT_DIR"
tar -czvf "$RELEASE_DIR"/"$binary_name" -- *
popd
rm -rf "{$OUT_DIR:?}"/*

pushd "$RELEASE_DIR"
sha256sum "$binary_name" > "$RELEASE_DIR"/"$binary_name".sha256sum
popd
