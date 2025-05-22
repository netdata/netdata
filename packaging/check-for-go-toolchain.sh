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
GOLANG_MIN_MINOR_VERSION='24'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="4c382776d52313266f3026236297a224a6688751256a2dffa3f524d8d6f6c0ba"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="68097bd680839cbc9d464a0edce4f7c333975e27a90246890e9f1078c7e702ad"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="756274ea4b68fa5535eb9fe2559889287d725a8da63c6aae4d5f23778c229f4b"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="438d5d3d7dcb239b58d893a715672eabe670b9730b1fd1c8fc858a46722a598a"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="5fff857791d541c71d8ea0171c73f6f99770d15ff7e2ad979104856d01f36563"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="91bda1558fcbd1c92769ad86c8f5cf796f8c67b0d9d9c19f76eecfc75ce71527"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1cb3448166d6abb515a85a3ee5afbdf932081fb58ad7143a8fb666fbc06146d9"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="026f1dd906189acff714c7625686bbc4ed91042618ba010d45b671461acc9e63"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="49399ba759b570a8f87d12179133403da6c2dd296d63a8830dee309161b9c40c"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="1f48f47183794d97c29736004247ab541177cf984ac6322c78bc43828daa1172"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ef856428b60a8c0bd9a2cba596e83024be6f1c2d5574e89cb1ff2262b08df8b9"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.24.2.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="ec2088823e16df00600a6d0f72e9a7dc6d2f80c9c140c2043c0cf20e1404d1a9"
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
