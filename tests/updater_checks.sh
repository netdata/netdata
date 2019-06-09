#!/usr/bin/env sh
#
# Wrapper script that installs the required dependencies
# for the BATS script to run successfully
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)
#

echo "Syncing/updating repository.."

blind_arch_grep_install() {
	# There is a peculiar docker case with arch, where grep is not available
	# This method will have to be triggered blindly, to inject grep so that we can process
	# It starts to become a chicken-egg situation with all the distros..
	echo "* * Workaround hack * *"
	echo "Attempting blind install for archlinux case"

	if command -v pacman > /dev/null 2>&1; then
		echo "Executing grep installation"
		pacman -Sy
		pacman --noconfirm --needed -S grep
	fi
}
blind_arch_grep_install || echo "Workaround failed, proceed as usual"

running_os="$(cat /etc/os-release |grep '^ID=' | cut -d'=' -f2 | sed -e 's/"//g')"

case "${running_os}" in
"centos"|"fedora")
	echo "Running on CentOS, updating YUM repository.."
	yum clean all
	yum update -y

	echo "Installing extra dependencies.."
	yum install -y epel-release
	yum install -y bats curl
	;;
"debian"|"ubuntu")
	echo "Running ${running_os}, updating APT repository"
	apt-get update -y
	apt-get install -y bats curl
	;;
"opensuse-leap"|"opensuse-tumbleweed")
	zypper update -y
	zypper install -y bats curl
	;;
"arch")
	pacman -Sy
	pacman --noconfirm --needed -S bash-bats curl
	;;
"alpine")
	apk update
	apk add bash curl bats
	;;
*)
	echo "Running on ${running_os}, no repository preparation done"
	;;
esac

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

echo "Running BATS file.."
bats --tap tests/updater_checks.bats
