#!/bin/sh
#
# Copyright (c) 2024 Netdata Inc.
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
GOLANG_MIN_MINOR_VERSION='23'
GOLANG_MIN_PATCH_VERSION='3'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="3d7b00191a43c50d28e0903a0c576104bc7e171a8670de419d41111c08dfa299"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="a0afb9744c00648bafb1b90b4aba5bdb86f424f02f9275399ce0c20b93a2c3a8"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1f7cbd7f668ea32a107ecd41b6488aaee1f5d77a66efd885b175494439d4e1ce"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5f0332754beffc65af65a7b2da76e9dd997567d0d81b6f4f71d3588dc7b4cb00"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e3b926c81e8099d3cee6e6e270b85b39c3bd44263f8d3df29aacb4d7e00507c8"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="324e03b6f59be841dfbaeabc466224b0f0905f5ad3a225b7c0703090e6c4b1a5"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="6bd72fcef72b046b6282c2d1f2c38f31600e4fe9361fcd8341500c754fb09c38"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="69479fa016ec5b4605885643ce0c2dd5c583e02353978feb6de38c961863b9cc"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="bf1de22a900646ef4f79480ed88337856d47089cc610f87e6fef46f6b8db0e1f"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e461f866479bc36bdd4cfec32bfecb1bb243152268a1b3223de109410dec3407"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="24154b4018a45540aefeb6b5b9ffdcc8d9a8cdb78cd7fec262787b89fed19997"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.3.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="218f3f1532e61dd65c330c2a5fc85bec18cc3690489763e62ffa9bb9fc85a68e"
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
