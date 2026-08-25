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

# Report the machine type of the *userland* we are building for.
#
# `uname -m` reports the kernel's architecture, which is not the same thing: a
# 32-bit userland on a 64-bit kernel (i386 on x86_64, armhf on aarch64 - the
# default on 32-bit Raspberry Pi OS) reports the 64-bit kernel type. Installing a
# 64-bit Go toolchain there makes cgo build for the wrong architecture.
userland_machine() {
  ULM_KERNEL_MACHINE="$(uname -m)"

  # Speak one vocabulary from here on: the Linux archive table below is keyed by
  # the kernel spellings, so fold the Debian/BSD aliases into them first. A Linux
  # kernel reports x86_64/aarch64 (i686/armv7l/armv8l for a 32-bit personality),
  # never these aliases, but folding them once keeps every branch below - the
  # word-size gate, the 32-bit map, and the messages - consistent.
  case "${ULM_KERNEL_MACHINE}" in
    amd64) ULM_KERNEL_MACHINE=x86_64 ;;
    arm64) ULM_KERNEL_MACHINE=aarch64 ;;
  esac

  # Only 64-bit kernel types that install_go_toolchain can resolve to an archive
  # need the userland check: every other machine type either cannot host a
  # narrower userland, or has no Go archive at all and already fails cleanly.
  case "${ULM_KERNEL_MACHINE}" in
    x86_64|aarch64|ppc64le|riscv64|s390x) ;;
    *)
      printf '%s\n' "${ULM_KERNEL_MACHINE}"
      return 0
      ;;
  esac

  # Userland word size. getconf is not present everywhere (notably minimal musl
  # systems), so fall back to the ELF class byte of a userland binary - od's own
  # executable, which /proc/self/exe resolves to inside the substitution below:
  # byte 4 of the ELF header is 1 for 32-bit objects and 2 for 64-bit ones.
  ULM_BITS=''
  if command -v getconf > /dev/null 2>&1; then
    ULM_BITS="$(getconf LONG_BIT 2>/dev/null)"
  fi

  if [ -z "${ULM_BITS}" ] && [ -r /proc/self/exe ] && command -v od > /dev/null 2>&1; then
    case "$(od -An -t x1 -j 4 -N 1 /proc/self/exe 2>/dev/null | tr -d ' \n')" in
      01) ULM_BITS=32 ;;
      02) ULM_BITS=64 ;;
    esac
  fi

  case "${ULM_BITS}" in
    32)
      # Map the 64-bit kernel type to the 32-bit userland we are building for. A
      # kernel type with no 32-bit Go toolchain (Go ships none for ppc64le,
      # s390x or riscv64 userlands) gets a marker that makes the caller fail
      # cleanly, instead of installing a 64-bit toolchain that cannot run here.
      case "${ULM_KERNEL_MACHINE}" in
        x86_64) printf '%s\n' i686 ;;
        aarch64) printf '%s\n' armv7l ;;
        *) printf '%s\n' "32-bit userland on ${ULM_KERNEL_MACHINE}" ;;
      esac
      ;;
    64)
      printf '%s\n' "${ULM_KERNEL_MACHINE}"
      ;;
    *)
      # Word size undetermined (no getconf, and no readable ELF header to fall
      # back to). Assume the userland matches the kernel, but say so: if it does
      # not, the toolchain we are about to install will not run.
      printf '%s\n' "WARNING: cannot determine the userland word size (getconf and od are both unavailable), assuming a 64-bit userland on this ${ULM_KERNEL_MACHINE} kernel. If the userland is 32-bit, install a Go ${GOLANG_MIN_VERSION} toolchain for it manually before building." >&2
      printf '%s\n' "${ULM_KERNEL_MACHINE}"
      ;;
  esac
}

install_go_toolchain() {
  GOLANG_ARCHIVE_NAME="${GOLANG_TEMP_PATH}/golang.tar.gz"
  GOLANG_CHECKSUM_FILE="${GOLANG_TEMP_PATH}/golang.sha256sums"

  case "$(uname -s)" in
    Linux)
      # The toolchain we download has to *run* here, so this is the host
      # userland, spelled the way Linux `uname -m` spells it. FreeBSD keeps
      # `uname -m` because its case labels (386/amd64/arm/arm64) are a different
      # vocabulary.
      #
      # Deliberately not derived from GOARCH: that is Go's *output* target, and
      # honouring it here would install a toolchain the builder cannot execute
      # whenever someone genuinely cross-builds. Once a runnable toolchain
      # exists, GOARCH does its job unaided.
      GOLANG_HOST_MACHINE="$(userland_machine)"

      case "${GOLANG_HOST_MACHINE}" in
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
          GOLANG_FAILURE_REASON="Linux ${GOLANG_HOST_MACHINE} platform is not supported out-of-box by Go, you must install a toolchain for it yourself."
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
