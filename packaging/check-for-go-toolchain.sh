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
GOLANG_MIN_PATCH_VERSION='7'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="35e2ec7a7ae6905a1fae5459197b70e3fcbc5e0a786a7d6ba8e49bcd38ad2e26"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="bd03b743eb6eb4193ea3c3fd3956546bf0e3ca5b7076c8226334afe6b75704cd"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="3f6b48d96f0d8dff77e4625aa179e0449f6bbe79b6986bfa711c2cfc1257ebd8"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="3066b2284b554da76cf664d217490792ba6f292ec0fc20bf9615e173cc0d2800"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ab9226ecddda0f682365c949114b653a66c2e9330e7b8d3edea80858437d2ff2"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="d62137f11530b97f3503453ad7d9e570af070770599fb8054f4e8cd0e905a453"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="9f07792e085f0d212c75ba403cb73e7f2f71eace48a38fab58711270dd7b1cef"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7bba5a430d2c562af87b6c1a31cccf72c43107b7318b48aa8a02441df61acd08"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="fe15a74bdb33954ebc9312efb01ac1871f7fc9cc712993058de8fc2a4dc8c8f7"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5d92e2d65a543811dca9f76a2b533cbdc051bdd5015bf789b137e2dcc33b2d52"
          ;;
# broken: https://go.dev/doc/go1.26#freebsd
#        riscv64)
#          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.0.freebsd-riscv64.tar.gz"
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
