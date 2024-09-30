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
GOLANG_MIN_PATCH_VERSION='1'
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

    if [ -n "${version_patch}" ] &&  [ "${version_patch}" -ge "${GOLANG_MIN_PATCH_VERSION}" ]; then
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="cdee2f4e2efa001f7ee75c90f2efc310b63346cfbba7b549987e9139527c6b17"
                    ;;
                x86_64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="49bbb517cfa9eee677e1e7897f7cf9cfdbcf49e05f61984a2789136de359f9bd"
                    ;;
                aarch64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="faec7f7f8ae53fda0f3d408f52182d942cc89ef5b7d3d9f23ff117437d4b2d2f"
                    ;;
                armv*)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-armv6l.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="6c7832c7dcd8fb6d4eb308f672a725393403c74ee7be1aeccd8a443015df99de"
                    ;;
                ppc64le)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-ppc64le.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="042888cae54b5fbfd9dd1e3b6bc4a5134879777fe6497fc4c62ec394b5ecf2da"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="1a4a609f0391bea202d9095453cbfaf7368fa88a04c206bf9dd715a738664dc3"
                    ;;
                s390x)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.linux-s390x.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="47dc49ad45c45e192efa0df7dc7bc5403f5f2d15b5d0dc74ef3018154b616f4d"
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.freebsd-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="cc957c1a019702e6cdc2e257202d42799011ebc1968b6c3bcd6b1965952607d5"
                    ;;
                amd64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.freebsd-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="a7d57781c50bb80886a8f04066791956d45aa3eea0f83070c5268b6223afb2ff"
                    ;;
                arm)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.freebsd-arm.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="c7b09f3fef456048e596db9bea746eb66796aeb82885622b0388feee18f36a3e"
                    ;;
                arm64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.freebsd-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="b05cd6a77995a0c8439d88df124811c725fb78b942d0b6dd1643529d7ba62f1f"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.1.freebsd-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="56236ae70be1613f2915943b94f53c96be5bffc0719314078facd778a89bc57e"
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

    echo "${GOLANG_ARCHIVE_CHECKSUM}  ${GOLANG_ARCHIVE_NAME}" > "${GOLANG_CHECKSUM_FILE}"

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
