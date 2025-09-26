#!/bin/sh
# SPDX-License-Identifier: GPL-v3+
#
# Check if we need to install a Go toolchain.
#
# Scripts that use this should call the ensure_go_toolchain function
# after sourcing this file to handle things correctly.
#
# If a working Go toolchain is either present or was installed, then the
# function will return 0. If a working Go toolchain is not present and one
# cannot be installed, then it will instead return 1, with the variable
# GOLANG_FAILURE_REASON set to an error message indicating what went wrong.

GOLANG_MIN_MAJOR_VERSION='1'
GOLANG_MIN_MINOR_VERSION='25'
GOLANG_MIN_PATCH_VERSION='0'
GOLANG_MIN_VERSION="${GOLANG_MIN_MAJOR_VERSION}.${GOLANG_MIN_MINOR_VERSION}.${GOLANG_MIN_PATCH_VERSION}"

GOLANG_TEMP_PATH="${TMPDIR}/go-toolchain"

check_go_version() {
  version="$("${go}" version | awk '{ print $3 }' | sed 's/^go//')"
  version_major="$(echo "${version}" | cut -f 1 -d '.')"
  version_minor="$(echo "${version}" | cut -f 2 -d '.')"
  version_patch="$(echo "${version}" | cut -f 3 -d '.')"

  if [ -z "${version_major}" ] || [ "${version_major}" -lt "${GOLANG_MIN_MAJOR_VERSION}" ]; then
    return 1
  elif [ "${version_major}" -gt "${GOLANG_MIN_MAJOR_VERSION}" ]; then
    return 0
  fi

  if [ -z "${version_minor}" ] || [ "${version_minor}" -lt "${GOLANG_MIN_MINOR_VERSION}" ]; then
    return 1
  elif [ "${version_minor}" -gt "${GOLANG_MIN_MINOR_VERSION}" ]; then
    return 0
  fi

  if [ -n "${version_patch}" ] && [ "${version_patch}" -ge "${GOLANG_MIN_PATCH_VERSION}" ]; then
    return 0
  fi

  return 1
}

install_go_toolchain() {
  GOLANG_ARCHIVE_NAME="${GOLANG_TEMP_PATH}/golang.tar.gz"
  GOLANG_CHECKSUM_FILE="${GOLANG_TEMP_PATH}/golang.sha256sums"

  case "$(uname -s)" in
    Linux)
      case "$(uname -m)" in
        i?86)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="d03cdcbc9bd8baf5cf028de390478e9e2b3e4d0afe5a6582dedc19bfe6a263b2"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7716a0d940a0f6ae8e1f3b3f4f36299dc53e31b16840dbd171254312c41ca12e"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="65a3e34fb2126f55b34e1edfc709121660e1be2dee6bdf405fc399a63a95a87d"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="eb949be683e82a99e9861dafd7057e31ea40b161eae6c4cd18fdc0e8c4ae6225"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="8b0c8d3ee5b1b5c28b6bd63dc4438792012e01d03b4bf7a61d985c87edab7d1f"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="22fe934a9d0c9c57275716c55b92d46ebd887cec3177c9140705efa9f84ba1e2"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="9cfe517ba423f59f3738ca5c3d907c103253cffbbcc2987142f79c5de8c1bf93"
          ;;
        *)
          GOLANG_FAILURE_REASON="Linux $(uname -m) platform is not supported out-of-box by Go, you must install a toolchain for it yourself."
          return 1
          ;;
      esac
      ;;
    FreeBSD)
      case "$(uname -m)" in
        386)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="dc0198dd4ec520e13f26798def8750544edf6448d8e9c43fd2a814e4885932af"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="c4f1a7e7b258406e6f3b677ecdbd97bbb23ff9c0d44be4eb238a07d360f69ac8"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7772fc5ff71ed39297ec0c1599fc54e399642c9b848eac989601040923b0de9c"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5bb011d5d5b6218b12189f07aa0be618ab2002662fff1ca40afba7389735c207"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.1.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ccac716240cb049bebfafcb7eebc3758512178a4c51fc26da9cc032035d850c8"
          ;;
        *)
          GOLANG_FAILURE_REASON="FreeBSD $(uname -m) platform is not supported out-of-box by Go, you must install a toolchain for it yourself."
          return 1
          ;;
      esac
      ;;
    *)
      GOLANG_FAILURE_REASON="We do not support automatic handling of a Go toolchain on this system, you must install one manually."
      return 1
      ;;
  esac

  if [ -d '/usr/local/go' ]; then
    if [ -f '/usr/local/go/.installed-by-netdata' ]; then
      rm -rf /usr/local/go
    else
      GOLANG_FAILURE_REASON="Refusing to overwrite existing Go toolchain install at /usr/local/go, it needs to be updated manually."
      return 1
    fi
  fi

  mkdir -p "${GOLANG_TEMP_PATH}"

  if ! curl --fail -q -sSL --connect-timeout 10 --retry 3 --output "${GOLANG_ARCHIVE_NAME}" "${GOLANG_ARCHIVE_URL}"; then
    GOLANG_FAILURE_REASON="Failed to download Go toolchain."
    return 1
  fi

  echo "${GOLANG_ARCHIVE_CHECKSUM}  ${GOLANG_ARCHIVE_NAME}" >"${GOLANG_CHECKSUM_FILE}"

  if ! sha256sum -c "${GOLANG_CHECKSUM_FILE}"; then
    GOLANG_FAILURE_REASON="Invalid checksum for downloaded Go toolchain."
    return 1
  fi

  if ! tar -C /usr/local/ -xzf "${GOLANG_ARCHIVE_NAME}"; then
    GOLANG_FAILURE_REASON="Failed to extract Go toolchain."
    return 1
  fi

  touch /usr/local/go/.installed-by-netdata

  rm -rf "${GOLANG_TEMP_PATH}"
}

ensure_go_toolchain() {
  go="$(PATH="/usr/local/go/bin:${PATH}" command -v go 2>/dev/null)"

  need_go_install=0

  if [ -z "${go}" ]; then
    need_go_install=1
  elif ! check_go_version; then
    need_go_install=1
  fi

  if [ "${need_go_install}" -eq 1 ]; then
    if ! install_go_toolchain; then
      return 1
    fi

    rm -rf "${GOLANG_TEMP_PATH}" || true
  fi

  return 0
}
