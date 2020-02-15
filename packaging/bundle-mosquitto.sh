#!/bin/sh

MOSQUITTO_TARBALL="$(cat "${1}/packaging/mosquitto.version").tar.gz"
MOSQUITTO_BUILD_PATH="${1}/externaldeps/mosquitto/mosquitto-$(cat "${1}/packaging/mosquitto.version")/lib"

mkdir -p "${1}/externaldeps/mosquitto" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/mosquitto/archive/${MOSQUITTO_TARBALL}" > "${MOSQUITTO_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/mosquitto.checksums" || exit 1
tar -xzf "${MOSQUITTO_TARBALL}" -C "${1}/externaldeps/mosquitto" || exit 1
make -C "${MOSQUITTO_BUILD_PATH}" || exit 1
cp -a "${MOSQUITTO_BUILD_PATH}/libmosquitto.a" "${1}/externaldeps/mosquitto" || exit 1
cp -a "${MOSQUITTO_BUILD_PATH}/mosquitto.h" "${1}/externaldeps/mosquitto" || exit 1
