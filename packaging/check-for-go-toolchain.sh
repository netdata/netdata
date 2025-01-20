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
GOLANG_MIN_PATCH_VERSION='5'
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="6ecf6a41d0925358905fa2641db0e1c9037aa5b5bcd26ca6734caf50d9196417"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="cbcad4a6482107c7c7926df1608106c189417163428200ce357695cc7e01d091"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="47c84d332123883653b70da2db7dd57d2a865921ba4724efcdf56b5da7021db0"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="04e0b5cf5c216f0aa1bf8204d49312ad0845800ab0702dfe4357c0b1241027a3"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="db268bf5710b5b1b82ab38722ba6e4427d9e4942aed78c7d09195a9dff329613"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="d9da15778442464f32acfa777ac731fd4d47362b233b83a0932380cb6d2d5dc8"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="14924b917d35311eb130e263f34931043d4f9dc65f20684301bf8f60a72edcdf"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7204e7bc62913b12f18c61afe0bc1a92fd192c0e45a54125978592296cb84e49"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="90a119995ebc3e36082874df5fa8fe6da194946679d01ae8bef33c87aab99391"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="255d26d873e41ff2fc278013bb2e5f25cf2ebe8d0ec84c07e3bb1436216020d3"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="2785d9122654980b59ca38305a11b34f2a1e12d9f7eb41d52efc137c1fc29e61"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.5.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="8f66a94018ab666d56868f61c579aa81e549ac9700979ce6004445d315be2d37"
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
