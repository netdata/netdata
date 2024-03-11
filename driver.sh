#!/usr/bin/env bash

set -exu -o pipefail

#
# Build the debs
#
git clean -xfd . && driver -r debs

pushd build
cpack -G DEB
popd

#
# Extract the artifacts
#
pushd artifacts

declare -A packages

packages=(
    [netdata]=netdata-netdata_6.7.8_amd64.deb
    [cups_plugin]=netdata-cups_plugin_6.7.8_amd64.deb
    [debugfs_plugin]=netdata-debugfs_plugin_6.7.8_amd64.deb
    [slabinfo_plugin]=netdata-slabinfo_plugin_6.7.8_amd64.deb
    [xenstat_plugin]=netdata-xenstat_plugin_6.7.8_amd64.deb
)

for key in "${!packages[@]}"; do
    package=${packages[$key]}
    dpkg-deb -R "$package" "$key"
done

popd
