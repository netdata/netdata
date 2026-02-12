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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="db908a86e888574ed3432355ba5372ad3ef2c0821ba9b91ceaa0f6634620c40c"
          ;;
        x86_64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="9e9b755d63b36acf30c12a9a3fc379243714c1c6d3dd72861da637f336ebb35b"
          ;;
        aarch64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="b00b694903d126c588c378e72d3545549935d3982635ba3f7a964c9fa23fe3b9"
          ;;
        armv*)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-armv6l.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="0b27e3dec8d04899d6941586d2aa2721c3dee67c739c1fc1b528188f3f6e8ab5"
          ;;
        ppc64le)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-ppc64le.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="f0904b647b5b8561efc5d48bb59a34f2b7996afab83ccd41c93b1aeb2c0067e4"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="05de84b319bc91b9cecbc6bf8eb5fcd814cf8a9d16c248d293dbd96f6cc0151b"
          ;;
        s390x)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.linux-s390x.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="a5d0a72b0dfd57f9c2c0cdd8b7e0f401e0afb9e8c304d3410f9b0982ce0953da"
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
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.freebsd-386.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="f8ff9fa5309fbbbd7d52f5d3f7181feb830dfd044d23c38746a2ada091f751b5"
          ;;
        amd64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.freebsd-amd64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="a2d2b2aeb218bd646fd8708bacc96c9d4de1b6c9ea48ceb9171e9e784f676650"
          ;;
        arm)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.freebsd-arm.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="b83a5cb1695c7185a13840661aef6aa1b46202d41a72528ecde51735765c6641"
          ;;
        arm64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.freebsd-arm64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="938fc0204f853c24ab03967105146af6590903dd14f869fe912db7a735f654f6"
          ;;
        riscv64)
          GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.25.5.freebsd-riscv64.tar.gz"
          GOLANG_ARCHIVE_CHECKSUM="7b0cc61246cf6fc9e576135cfcd2b95e870b0f2ee5bf057325b2d76119001e4e"
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
