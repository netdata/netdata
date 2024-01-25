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
GOLANG_MIN_MINOR_VERSION='21'
GOLANG_MIN_PATCH_VERSION='0'

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

    if [ "$(uname -s)" != "Linux" ]; then
        GOLANG_FAILURE_REASON="We do not support automatic handling of a Go toolchain on this system, you must install one manually."
        return 1
    fi

    case "$(uname -m)" in
        i?86)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-386.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="05d09041b5a1193c14e4b2db3f7fcc649b236c567f5eb93305c537851b72dd95"
            ;;
        x86_64)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-amd64.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="3f934f40ac360b9c01f616a9aa1796d227d8b0328bf64cb045c7b8c4ee9caea4"
            ;;
        aarch64)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-arm64.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="e2e8aa88e1b5170a0d495d7d9c766af2b2b6c6925a8f8956d834ad6b4cacbd9a"
            ;;
        armv*)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-armv6l.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="6a8eda6cc6a799ff25e74ce0c13fdc1a76c0983a0bb07c789a2a3454bf6ec9b2"
            ;;
        ppc64le)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-ppc64le.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="e872b1e9a3f2f08fd4554615a32ca9123a4ba877ab6d19d36abc3424f86bc07f"
            ;;
        riscv64)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-riscv64.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="86a2fe6597af4b37d98bca632f109034b624786a8d9c1504d340661355ed31f7"
            ;;
        s390x)
            GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.21.6.linux-s390x.tar.gz"
            GOLANG_ARCHIVE_CHECKSUM="92894d0f732d3379bc414ffdd617eaadad47e1d72610e10d69a1156db03fc052"
            ;;
        *)
            GOLANG_FAILURE_REASON="Linux $(uname -m) platform is not supported out-of-box by Go, you must install a toolchain for it yourself."
            return 1
            ;;
    esac

    if [ -d '/usr/local/go' ]; then
        GOLANG_FAILURE_REASON="Refusing to overwrite existing Go toolchain install at /usr/local/go, it needs to be updated manually."
        return 1
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
