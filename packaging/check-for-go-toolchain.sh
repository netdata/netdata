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
GOLANG_MIN_MINOR_VERSION='23'
GOLANG_MIN_PATCH_VERSION='4'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="4a4a0e7587ef8c8a326439b957027f2791795e2d29d4ae3885b4091a48f843bc"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="6924efde5de86fe277676e929dc9917d466efa02fb934197bc2eba35d5680971"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="16e5017863a7f6071363782b1b8042eb12c6ca4f4cd71528b2123f0a1275b13e"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1f1dda0dc7ce0b2295f57258ec5ef0803fd31b9ed0aa20e2e9222334e5755de1"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="65a303ef51e48ff77e004a6a5b4db6ce59495cd59c6af51b54bf4f786c01a1b9"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7c40e9e0d722cef14ede765159ba297f4c6e3093bb106f10fbccf8564780049a"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="74aab82bf4eca7c26c830a5b0e2a31d193a4d5ba47045526b92473cc7188d7d7"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="8df26b1e71234756c1f0e82cfffba3f427c5a91a251844ada2c7694a6986c546"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7de078d94d2af50ee9506ef7df85e4d12d4018b23e0b2cbcbc61d686f549b41a"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="3f23e0a01cfe24e4160124cd7ab02bdd188264652074abdbce401c93f41e58a4"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="986a20e7c94431f03b44b3c415abc698c7b4edc2ae8431f7ecae1c2429d4cfa2"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.4.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="25e39f005f977778ce963fc43089510fe7514f3cfc0358eab584de4ce9f181fb"
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
