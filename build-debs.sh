#!/usr/bin/env bash

set -exu -o pipefail

#
# Build the debs
#
rm -rf artifacts/

./driver.sh -r cpack

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
