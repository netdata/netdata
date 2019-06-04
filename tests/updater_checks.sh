#!/usr/bin/env bash
#
# Wrapper script that installs the required dependencies
# for the BATS script to run successfully
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)
#

echo "Syncing/updating repository.."
running_os="$(cat /etc/os-release |grep '^ID=' | cut -d'=' -f2 | sed -e 's/"//g')"

case "${running_os}" in
"centos"|"fedora")
	echo "Running on CentOS, updating YUM repository.."
	yum clean all
	yum update -y

	echo "Installing extra dependencies.."
	yum install -y epel-release
	yum install -y git bats curl
	;;
"debian"|"ubuntu")
	echo "Running ${running_os}, updating APT repository"
	apt-get update -y
	apt-get install -y git bats curl
	;;
*)
	echo "Running on ${running_os}, no repository preparation done"
	;;
esac

echo "Running BATS file.."
bats --tap tests/updater_checks.bats
