#!/usr/bin/env bash

# Netdata
# Monitoring and troubleshooting, transformed!
# Copyright 2018-2025 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Script to test alarm notifications for netdata

dir="$(dirname "${0}")"
"${dir}/alarm-notify.sh" test "${1}"
exit $?
