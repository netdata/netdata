# no-shebang-needed-its-a-library
#
# Utility functions for packaging in travis CI
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
#shellcheck disable=SC2148
set -e

function detect_arch_from_commit {
	case "${TRAVIS_COMMIT_MESSAGE}" in
		"[Package amd64"*)
			export BUILD_ARCH="amd64"
			;;
		"[Package i386"*)
			export BUILD_ARCH="i386"
			;;
		"[Package ALL"*)
			export BUILD_ARCH="all"
			;;
		"[Package arm64"*)
			export BUILD_ARCH="arm64"
			;;

		*)
			echo "Unknown build architecture in '${TRAVIS_COMMIT_MESSAGE}'. No BUILD_ARCH can be provided"
			exit 1
			;;
	esac

	echo "Detected build architecture ${BUILD_ARCH}"
}
