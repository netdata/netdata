#!/bin/sh

H2O_PATH="${1}/httpd/h2o"

OLDPWD="${PWD}"
cd "${PROTOBUF_BUILD_PATH}" || exit 1
mkdir -p "${H2O_PATH}/build" || exit 1
cd "${H2O_PATH}/build" || exit 1
cmake -DWITHOUT_LIBS=OFF -DBUILD_SHARED_LIBS=OFF -DWITH_MRUBY=OFF .. || exit 1
make || exit 1
cd "${OLDPWD}" || exit 1
