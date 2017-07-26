#!/usr/bin/env bash

# netdata
# real-time performance and health monitoring, done right!
# (C) 2017 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#
# Script to test alarm notifications for netdata

dir="$(dirname "${0}")"
${dir}/alarm-notify.sh test "${1}"
exit $?
