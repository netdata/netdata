#!/usr/bin/env bash

DISTRO="$1"
VERSION="$2"

docker build -f make-install.Dockerfile -t "${DISTRO}_${VERSION}_dev:latest" .. \
       --build-arg "DISTRO=${DISTRO}" --build-arg "VERSION=${VERSION}"
