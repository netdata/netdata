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
GOLANG_MIN_MINOR_VERSION='26'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5ca0982791791559d11a0eba939617a94c3f37c21aa514a55c415b9167efc658"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1153d3d50e0ac764b447adfe05c2bcf08e889d42a02e0fe0259bd47f6733ad7f"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ef758ae7c6cf9267c9c0ef080b8965f453d89ab2d25d9eb22de4405925238768"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="8db458e995f18a9427a745cefe7a3323962fa2548c4715148963311f300d3b1a"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="53f49b8c7eace2d30389327b4a516b13321f90377fdf5929a6b63174609bc22e"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="2b9ba137baaa3031fd74330cb36ab54c5abe380867ca9fbab7c552f3db740555"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1e12a2318f3dd4c57027c865f6cb51f5d1e36b02d10f3e6316ce7b61a27b3ca1"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="f4325e0f9a5894e79c60b5e692af55a1e6f261b8ea73dcd20eced8372ad08233"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e91e96c0d3bd8f770e5a0facaf21d010a84e1c9440d830210cb3c223f0b7fb50"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="36e45b3a843087f0f95a71f2ed453ea7e1727e684644d13f8b4c6c93fd977d41"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="34aee1071d74cc23703d1b415c5fa44f840d29fd3733dac34944d83cfada3ff3"
          ;;
# broken: https://go.dev/doc/go1.26#freebsd
#        riscv64)
#          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.4.freebsd-riscv64.tar.gz"
#          GOLANG_ARCHIVE_CHECKSUM="7b0cc61246cf6fc9e576135cfcd2b95e870b0f2ee5bf057325b2d76119001e4e"
#          ;;
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

  if ! tar -C /usr/local/ --no-same-owner -xzf "${GOLANG_ARCHIVE_NAME}"; then
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
