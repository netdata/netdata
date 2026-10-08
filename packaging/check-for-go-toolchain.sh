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
GOLANG_MIN_MINOR_VERSION='27'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="eac4abaca4113170a1cf261b8bf1d38480e61e99deecbc6a14767deb8b19e8ad"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="675c26c449cbb18fc24b74650de1eabbae6e16f64326fd85a283fb3b58280685"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="51798d2c42d0e1c6ed7fd9f48728b4193abac9e8aad6dbac2fe96a81f5909bda"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e337ecd9c321377c0d8832690c2cb10463447c0bd0e65e2e3413dfff63a3435b"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="068085e257a6014c036c723ca62943edbf48fd168b8d00a6d39527d10b656255"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="e631e2e1e5aec4979960787f5f8cdb549001bb144c279e134447e54ffe8bd2d3"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="cb5c89fa48cba4123c68aee62b217ca030f281546df0898a0c32889c340b928c"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="db350b8961e4b01e0d0f3d4e4f98d6218bdab84ca112dedb27f1463c887d437f"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="2c24ddcca6dbd5e0be775a48cec4171e9ca616e654b63b02c9cbc515238dae31"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="79d202335f72966d4bd0ba3ac6b4e584337afe60e8cff1ad608d2b04284e039a"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.27.0.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="0d33b993d26337cd4cedaa57d4d0fec71c4b578a5b47d8d08cb153eb337b27c7"
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
