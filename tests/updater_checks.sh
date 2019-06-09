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

# Download and run depednency scriptlet, before anything else
#
deps_tool="/tmp/deps_tool.$$.sh"
curl -Ss -o ${deps_tool} https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh
if [ -f "${deps_tool}" ]; then
	echo "Running dependency handling script.."
	chmod +x "${deps_tool}"
	${deps_tool} --non-interactive netdata
	rm -f "${deps_tool}"
	echo "Done!"
else
	echo "Failed to fetch dependency script, aborting the test"
	exit 1
fi

running_os="$(cat /etc/os-release |grep '^ID=' | cut -d'=' -f2 | sed -e 's/"//g')"

case "${running_os}" in
"centos"|"fedora")
	echo "Running on CentOS, updating YUM repository.."
	yum clean all
	yum update -y

	echo "Installing extra dependencies.."
	yum install -y epel-release
	yum install -y bats
	;;
"debian"|"ubuntu")
	echo "Running ${running_os}, updating APT repository"
	apt-get update -y
	apt-get install -y bats
	;;
"opensuse-leap")
	zypper update -y
	zypper install -y bats
	;;
"arch")
	pacman -Sy
	pacman -S bash-bats
	;;
*)
	echo "Running on ${running_os}, no repository preparation done"
	;;
esac

echo "Running BATS file.."
bats --tap tests/updater_checks.bats
