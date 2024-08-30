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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="1e209c4abde069067ac9afb341c8003db6a210f8173c77777f02d3a524313da3"
                    ;;
                x86_64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="f6c8a87aa03b92c4b0bf3d558e28ea03006eb29db78917daec5cfb6ec1046265"
                    ;;
                aarch64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="6a63fef0e050146f275bf02a0896badfe77c11b6f05499bb647e7bd613a45a10"
                    ;;
                armv*)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-armv6l.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="0525f92f79df7ed5877147bce7b955f159f3962711b69faac66bc7121d36dcc4"
                    ;;
                ppc64le)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-ppc64le.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="0e57f421df9449066f00155ce98a5be93744b3d81b00ee4c2c9b511be2a31d93"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="afe9cedcdbd6fdff27c57efd30aa5ce0f666f471fed5fa96cd4fb38d6b577086"
                    ;;
                s390x)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.linux-s390x.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="2e546a3583ba7bd3988f8f476245698f6a93dfa9fe206a8ca8f85c1ceecb2446"
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.freebsd-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="b8065da37783e8b9e7086365a54d74537e832c92311b61101a66989ab2458d8e"
                    ;;
                amd64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.freebsd-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="50f421c7f217083ac94aab1e09400cb9c2fea7d337679ec11f1638a11460da30"
                    ;;
                arm)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.freebsd-arm.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="c9c8b305f90903536f4981bad9f029828c2483b3216ca1783777344fbe603f2d"
                    ;;
                arm64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.freebsd-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="e23385e5c640787fa02cd58f2301ea09e162c4d99f8ca9fa6d52766f428a933d"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.22.0.freebsd-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="c8f94d1de6024546194d58e7b9370dc7ea06176aad94a675b0062c25c40cb645"
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
