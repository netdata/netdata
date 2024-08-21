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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="0e8a7340c2632e6fb5088d60f95b52be1f8303143e04cd34e9b2314fafc24edd"
                    ;;
                x86_64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="905a297f19ead44780548933e0ff1a1b86e8327bb459e92f9c0012569f76f5e3"
                    ;;
                aarch64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="62788056693009bcf7020eedc778cdd1781941c6145eab7688bd087bce0f8659"
                    ;;
                armv*)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-armv6l.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="0efa1338e644d7f74064fa7f1016b5da7872b2df0070ea3b56e4fef63192e35b"
                    ;;
                ppc64le)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-ppc64le.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="8b26e20d4d43a4d7641cddbdc0298d7ba3804d910a9e06cda7672970dbf2829d"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="a87726205f1a283247f877ccae8ce147ff4e77ac802382647ac52256eb5642c7"
                    ;;
                s390x)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.linux-s390x.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="003722971de02d97131a4dca2496abdab5cb175a6ee0ed9c8227c5ae9b883e69"
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
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.freebsd-386.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="2c9b76ead3c44f5b3e40e10b980075addb837f2dd05dafe7c0e4c611fd239753"
                    ;;
                amd64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.freebsd-amd64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="2c2252902b87ba605fdc0b12b4c860fe6553c0c5483c12cc471756ebdd8249fe"
                    ;;
                arm)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.freebsd-arm.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="8ec48b8d99a515644ae00e79d093ad3b7645dcaf2a19c0a9c0d97916187f4514"
                    ;;
                arm64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.freebsd-arm64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="f476bbe8efb0db18155671840545370bfb73903fec04ea897d510569dab16d9c"
                    ;;
                riscv64)
                    GOLANG_ARCHIVE_URL="https://go.dev/dl/go1.23.0.freebsd-riscv64.tar.gz"
                    GOLANG_ARCHIVE_CHECKSUM="b0e254b2ea5752b4f1c69934ae43a44bbabf98e0c2843af44e1b6d12390eb551"
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

    if [ "$(uname -m)" = "aarch64" ]; then
        GO_TELEMETRY_DIR="$HOME/.config/go/telemetry"
        [ ! -d "$GO_TELEMETRY_DIR" ] && mkdir -p "$GO_TELEMETRY_DIR"
        echo "off" >"$GO_TELEMETRY_DIR/mode"
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
