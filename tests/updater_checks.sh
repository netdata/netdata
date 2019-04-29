#!/usr/bin/env bash
#
# Wrapper script that installs the required dependencies
# for the BATS script to run successfully
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)
#

echo "Installing extra dependencies.."
yum install -y epel-release
yum install -y git bats

echo "Running BATS file.."
bats --tap tests/updater_checks.bats
