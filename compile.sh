#!/bin/sh

set -exu -o pipefail

export PATH="/usr/local/bin:${PATH}"

WT_ROOT="/cygdrive/c/Users/vk/repos/nd/master"
WT_PREFIX="/tmp"
BUILD_TYPE="Debug"

if [ -d "${WT_ROOT}/build" ]
then
	rm -rf "${WT_ROOT}/build"
fi

/usr/bin/cmake -S "${WT_ROOT}" -B "${WT_ROOT}/build" \
    -G Ninja \
    -DCMAKE_INSTALL_PREFIX="${WT_PREFIX}" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-Wall -Wextra -Wno-char-subscripts" \
    -DNETDATA_USER=vk \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_LOGS_MANAGEMENT_TESTS=Off \
    -DENABLE_ACLK=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
#    -DENABLE_MQTTWEBSOCKETS=On \


ninja -v -C build || ninja -v -C build -j 1
