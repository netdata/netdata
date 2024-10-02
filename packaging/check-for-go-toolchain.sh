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
GOLANG_MIN_MINOR_VERSION='22'
GOLANG_MIN_PATCH_VERSION='8'
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="0c8e9f824bf443f51e06ac017b9ae402ea066d761b309d880dbb2ca5793db8a2"
                    ;;
                x86_64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="5f467d29fc67c7ae6468cb6ad5b047a274bae8180cac5e0b7ddbfeba3e47e18f"
                    ;;
                aarch64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="5c616b32dab04bb8c4c8700478381daea0174dc70083e4026321163879278a4a"
                    ;;
                armv*)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-armv6l.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="5191e87a51a85d88edddc028ab30dfbfa2d7c37cf35d536655e7a063bfb2c9d2"
                    ;;
                ppc64le)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-ppc64le.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="c546f27866510bf8e54e86fe6f58c705af0e894341e5572c91f197a734152c27"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="f53174ee946b206afe66e043646a6f37af9375d5a9ce420c0f974790508f9e39"
                    ;;
                s390x)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.linux-s390x.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="fabb3adc241474e28ae151a00e1421983deb35184d31cc76e90025b1b389f6bf"
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.freebsd-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="854cffbfb089438397442be4a0c64239da50be4ed037606ea00ed8d86eb89514"
                    ;;
                amd64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.freebsd-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="d7dfa0b309d9ef9f63ad07c63300982ce3e658d7cbac20b031bd31e91afcf209"
                    ;;
                arm)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.freebsd-arm.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="5d532d05082524748f24948f3028c7a21e1804130ffd624bce4a3d0bee60ce39"
                    ;;
                arm64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.freebsd-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="f7d2664896ad6c773eafbab0748497bec62ff57beb4e25fe6dea12c443d05639"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.8.freebsd-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="ef7d2dbf341d8a8f2a15f2841216ef30329b1f5f301047bd256317480b22a033"
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
