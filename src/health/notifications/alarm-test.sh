#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Script to test alarm notifications for netdata

dir="$(dirname "${0}")"
"${dir}/alarm-notify.sh" test "${1}"
exit $?
