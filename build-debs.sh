#!/usr/bin/env bash

set -exu -o pipefail

#
# Build the debs
#
rm -rf artifacts/

./driver.sh -r debs

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
    [apps_plugin]=netdata-apps_plugin_6.7.8_amd64.deb
    [charts_d_plugin]=netdata-charts_d_plugin_6.7.8_amd64.deb
    [cups_plugin]=netdata-cups_plugin_6.7.8_amd64.deb
    [debugfs_plugin]=netdata-debugfs_plugin_6.7.8_amd64.deb
    [ebpf_plugin]=netdata-ebpf_plugin_6.7.8_amd64.deb
    [freeipmi_plugin]=netdata-freeipmi_plugin_6.7.8_amd64.deb
    [go_d_plugin]=netdata-go_d_plugin_6.7.8_amd64.deb
    [logs_management_plugin]=netdata-logs_management_plugin_6.7.8_amd64.deb
    [network_viewer_plugin]=netdata-network_viewer_plugin_6.7.8_amd64.deb
    [nfacct_plugin]=netdata-nfacct_plugin_6.7.8_amd64.deb
    [perf_plugin]=netdata-perf_plugin_6.7.8_amd64.deb
    [python_d_plugin]=netdata-python_d_plugin_6.7.8_amd64.deb
    [slabinfo_plugin]=netdata-slabinfo_plugin_6.7.8_amd64.deb
    [systemd_journal_plugin]=netdata-systemd_journal_plugin_6.7.8_amd64.deb
    [xenstat_plugin]=netdata-xenstat_plugin_6.7.8_amd64.deb
)

for key in "${!packages[@]}"; do
    package=${packages[$key]}
    dpkg-deb -R "$package" "$key"
done

popd
