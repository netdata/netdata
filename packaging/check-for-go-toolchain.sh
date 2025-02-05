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
GOLANG_MIN_PATCH_VERSION='6'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e61f87693169c0bbcc43363128f1e929b9dff0b7f448573f1bdd4e4a0b9687ba"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="9379441ea310de000f33a4dc767bd966e72ab2826270e038e78b2c53c2e7802d"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="561c780e8f4a8955d32bf72e46af0b5ee5e0debe1e4633df9a03781878219202"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="27a4611010c16b8c4f37ade3aada55bd5781998f02f348b164302fd5eea4eb74"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="0f817201e83d78ddbfa27f5f78d9b72450b92cc21d5e045145efacd0d3244a99"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="f95f7f817ab22ecab4503d0704d6449ea1aa26a595f57bf9b9f94ddf2aa7c1f3"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="321e7ed0d5416f731479c52fa7610b52b8079a8061967bd48cec6d66f671a60e"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="d3287706b5823712ac6cf7dff684a556cff98163ef60e7b275abe3388c17aac7"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ebb4c6a9b0673dbdabc439877779ed6add16575e21bd0a7955c33f692789aef6"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="b7241584afb0b161c09148f8fde16171bb743e47b99d451fbc5f5217ec7a88b6"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="004718b53cedd7955d1b1dc4053539fcd1053c031f5f3374334a22befd1f8310"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.6.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ca026ec8a30dd0c18164f40e1ce21bd725e2445f11699177d05815189a38de7a"
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
