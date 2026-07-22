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
GOLANG_MIN_PATCH_VERSION='2'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="88c162b204e6eefcc32499453b492e80209f4a4c78c33092636901c540fb0d05"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5c2c3b16caefa1d968a94c1daca04a7ca301a496d9b086e17ad77bb81393f053"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="fe4789e92b1f33358680864bbe8704289e7bb5fc207d80623c308935bd696d49"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="6dae9edab81c13bccf962dec15f1fd2ec26c14a6821b4d2c92dab4130c289d7a"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="c5d60e2b303bb612f20cd82786594b64874e73b35134025e27d3390bf284ae43"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="d4a24dd4484d3f86b99c2d300af0dea5d184557e6d61eb7aba19ff61662750e3"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="09ce3c504c0323968b75a717244dca4f25cd4cf0443e5ff6bc0bfa74add89fa7"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1a0226fc025d97d30a112ad0d09b13dcacedc5b24b04bf8f21a0cd29aac4d947"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="0e5ddc51a62018211d461d6bf409939b04eaa4d6dd6d7097910090ef755ed947"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="f8a59e86427158d89b2ba158d7f6004881e378fa3d7e4aefd4df17e4ee3a6bd1"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ae3825c8c57cc0e64c2233bfb9bba2e091f2126728e4c33492592c24b60dfcd0"
          ;;
# broken: https://go.dev/doc/go1.26#freebsd
#        riscv64)
#          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.26.5.freebsd-riscv64.tar.gz"
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
