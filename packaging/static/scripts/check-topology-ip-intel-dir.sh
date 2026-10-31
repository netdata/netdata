#!/bin/bash

set -euo pipefail

prefix="${1}"

files="
README.md
topology-ip-asn.mmdb
topology-ip-geo.mmdb
topology-ip-intel.json
"

for f in ${files} ; do
    p="${prefix}/usr/share/netdata/topology-ip-intel/${f}"
    if [ ! -s "${p}" ]; then
        echo "${p} is not present when it should be."
        exit 1
    fi
done
