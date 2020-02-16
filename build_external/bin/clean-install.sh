#!/usr/bin/env bash

DISTRO="$1"
VERSION="$2"
BuildBase="$(cd "$(dirname "$0")" && cd .. && pwd)"

# This is temporary - not all of the package-builder images from the helper-images repo
# are available on Docker Hub. When everything falls under the "happy case"  below this
# can be deleted in a future iteration. This is written in a weird way for portability,
# can't rely on bash 4.0+ to allow case fall-through with ;&

if cat <<HAPPY_CASE | grep "$DISTRO-$VERSION" 
    opensuse-15.1
    fedora-29
    debian-9
    debian-8
    fedora-30
    opensuse-15.0
    ubuntu-19.04
    centos-7
    fedora-31
    ubuntu-16.04
    ubuntu-18.04
    ubuntu-19.10
    debian-10
    centos-8
    ubuntu-1804
    ubuntu-1904
    ubuntu-1910
    debian-stretch
    debian-jessie
    debian-buster
HAPPY_CASE
then
    docker build -f "$BuildBase/clean-install.Dockerfile" -t "${DISTRO}_${VERSION}_dev" "$BuildBase/.." \
            --build-arg "DISTRO=$DISTRO" --build-arg "VERSION=$VERSION" --build-arg ACLK=yes \
            --build-arg EXTRA_CFLAGS="-DACLK_SSL_ALLOW_SELF_SIGNED"
else
    case "$DISTRO-$VERSION" in
        arch-current)
            docker build -f "$BuildBase/clean-install-arch.Dockerfile" -t "${DISTRO}_${VERSION}_dev" "$BuildBase/.." \
            --build-arg "DISTRO=$DISTRO" --build-arg "VERSION=$VERSION" --build-arg ACLK=yes \
            --build-arg EXTRA_CFLAGS="-DACLK_SSL_ALLOW_SELF_SIGNED"
        ;;
        *)
            echo "Unknown $DISTRO-$VERSION"
        ;;
    esac
fi
