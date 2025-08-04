#!/usr/bin/env bash

set -exu -o pipefail

cargo build -p otel-plugin
rm -f ~/opt/nom/netdata/usr/libexec/netdata/plugins.d/nom-plugin
cp $PWD/target/debug/nom-plugin ~/opt/nom/netdata/usr/libexec/netdata/plugins.d/nom-plugin
find ~/opt/nom/netdata/var/log -type f -delete
find ~/opt/nom/netdata/var/cache -type f -delete

cd /home/vk/repos/nd/nom
just run 19999
