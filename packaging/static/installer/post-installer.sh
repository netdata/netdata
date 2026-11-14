#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

# This script is started using the shell of the system
# and executes our 'install-or-update.sh' script
# using the netdata supplied, statically linked BASH
#
# so, at 'install-or-update.sh' we are always sure
# we run under BASH v4.

./bin/bash system/install-or-update.sh "${@}"
