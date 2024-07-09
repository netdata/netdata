#!/bin/bash

. /etc/profile

set -e

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

BUILD_DIR="${BUILD_DIR:-"${repo_root}/build"}"

cmake --install "${BUILD_DIR}"

makensis "${repo_root}/packaging/windows/installer.nsi"
