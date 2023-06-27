#!/bin/bash

if [ "$#" -eq 0 ]; then
    SENTRY_SOURCE="${PWD}/libnetdata/sentry/sentry-native"
    SENTRY_BUILD_PATH="${PWD}/externaldeps/sentry-native"
else
    SENTRY_SOURCE="${1}/libnetdata/sentry/sentry-native"
    SENTRY_BUILD_PATH="${2}/externaldeps/sentry-native"
fi

if [ -z "${ENABLE_SENTRY}" ]; then
    echo "Skipping sentry"
    return 0
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling sentry."

tmp="$(mktemp -d -t netdata-sentry-XXXXXX)"

if [ -d "${SENTRY_BUILD_PATH}" ]
then
    echo >&2 "Found compiled sentry-native, not compiling it again. Remove path ${SENTRY_BUILD_PATH}' to recompile."
    return 0
fi

cmake -B "${tmp}" \
    -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    -DCMAKE_INSTALL_LIBDIR="${SENTRY_BUILD_PATH}/lib" \
    -DCMAKE_INSTALL_PREFIX="${SENTRY_BUILD_PATH}" \
    -DBUILD_SHARED_LIBS=Off \
    -DCMAKE_C_FLAGS='-DJSMN_STATIC' \
    "${SENTRY_SOURCE}"

cmake --build "${tmp}" --parallel
cmake --install "${tmp}"

rm -rf "${tmp}"

echo "sentry built and prepared."
