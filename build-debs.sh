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
pushd artifacts

declare -A packages

packages=(
    [netdata]=netdata_6.7.8_amd64.deb
    [apps]=netdata-plugin-apps_6.7.8_amd64.deb
    [chartsd]=netdata-plugin-chartsd_6.7.8_all.deb
    [cups]=netdata-plugin-cups_6.7.8_amd64.deb
    [debugfs]=netdata-plugin-debugfs_6.7.8_amd64.deb
    [ebpf]=netdata-plugin-ebpf_6.7.8_amd64.deb
    [freeipmi]=netdata-plugin-freeipmi_6.7.8_amd64.deb
    [god]=netdata-plugin-go_6.7.8_amd64.deb
    [logs_management]=netdata-plugin-logs-management_6.7.8_amd64.deb
    [network_viewer]=netdata-plugin-network-viewer_6.7.8_amd64.deb
    [nfacct]=netdata-plugin-nfacct_6.7.8_amd64.deb
    [perf]=netdata-plugin-perf_6.7.8_amd64.deb
    [pythond]=netdata-plugin-pythond_6.7.8_all.deb
    [slabinfo]=netdata-plugin-slabinfo_6.7.8_amd64.deb
    [systemd_journal]=netdata-plugin-systemd-journal_6.7.8_amd64.deb
    [xenstat]=netdata-plugin-xenstat_6.7.8_amd64.deb
)

for key in "${!packages[@]}"; do
    package=${packages[$key]}
    dpkg-deb -R "$package" "$key"
done

popd
