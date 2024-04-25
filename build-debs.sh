#!/usr/bin/env bash

set -exu -o pipefail

#
# Build the debs
#

if [ -d build/ ]; then
    rm -rf build/packages/
    ./driver.sh -r cpack
else
    ninja -C build
fi

pushd build
cpack -G DEB
popd

#
# Extract the artifacts
#
pushd build/packages

for package in *.deb; do
    dpkg-deb -R "$package" "${package%.*}"
    dpkg-deb -x "$package" rfs
done

popd
