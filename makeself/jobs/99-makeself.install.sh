#!/usr/bin/env bash

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

run cd "${NETDATA_SOURCE_PATH}" || exit 1

# -----------------------------------------------------------------------------
# find the netdata version

NOWNER="unknown"
ORIGIN="$(git config --get remote.origin.url || echo "unknown")"
if [[ "${ORIGIN}" =~ ^git@github.com:.*/netdata.*$ ]]
    then
    NOWNER="${ORIGIN/git@github.com:/}"
    NOWNER="${NOWNER/\/netdata*/}"

elif [[ "${ORIGIN}" =~ ^https://github.com/.*/netdata.*$ ]]
    then
    NOWNER="${ORIGIN/https:\/\/github.com\//}"
    NOWNER="${NOWNER/\/netdata*/}"
fi

# make sure it does not have any slashes in it
NOWNER="${NOWNER//\//_}"

if [ "${NOWNER}" = "firehol" ]
    then
    NOWNER=
else
    NOWNER="-${NOWNER}"
fi

VERSION="$(git describe || echo "undefined")"
[ -z "${VERSION}" ] && VERSION="undefined"

FILE_VERSION="${VERSION}-$(uname -m)-$(date +"%Y%m%d-%H%M%S")${NOWNER}"


# -----------------------------------------------------------------------------
# copy the files needed by makeself installation

run mkdir -p "${NETDATA_INSTALL_PATH}/system"

run cp \
    makeself/post-installer.sh \
    makeself/install-or-update.sh \
    installer/functions.sh \
    configs.signatures \
    system/netdata-init-d \
    system/netdata-lsb \
    system/netdata-openrc \
    system/netdata.logrotate \
    system/netdata.service \
    "${NETDATA_INSTALL_PATH}/system/"


# -----------------------------------------------------------------------------
# create a wrapper to start our netdata with a modified path

run mkdir -p "${NETDATA_INSTALL_PATH}/bin/srv"

run mv "${NETDATA_INSTALL_PATH}/bin/netdata" \
    "${NETDATA_INSTALL_PATH}/bin/srv/netdata" || exit 1

cat >"${NETDATA_INSTALL_PATH}/bin/netdata" <<EOF
#!${NETDATA_INSTALL_PATH}/bin/bash
export BASH_LOADABLES_PATH="${NETDATA_INSTALL_PATH}/lib/bash"
export PATH="${NETDATA_INSTALL_PATH}/bin:\${PATH}"
exec "${NETDATA_INSTALL_PATH}/bin/srv/netdata" "\${@}"
EOF
run chmod 755 "${NETDATA_INSTALL_PATH}/bin/netdata"


# -----------------------------------------------------------------------------
# move etc to protect the destination when unpacked

if [ ! -z "${NETDATA_INSTALL_PATH}" -a -d "${NETDATA_INSTALL_PATH}/etc" ]
    then
    if [ -d "${NETDATA_INSTALL_PATH}/etc.new" ]
        then
        run rm -rf "${NETDATA_INSTALL_PATH}/etc.new" || exit 1
    fi

    run mv "${NETDATA_INSTALL_PATH}/etc" \
        "${NETDATA_INSTALL_PATH}/etc.new" || exit 1

    if [ -f "${NETDATA_INSTALL_PATH}/etc.new/netdata/netdata.conf" ]
        then
        # delete the generated netdata.conf, so that the static installer will generate a new one
        run rm "${NETDATA_INSTALL_PATH}/etc.new/netdata/netdata.conf"
    fi
fi


# -----------------------------------------------------------------------------
# remove the links to allow untaring the archive

run rm "${NETDATA_INSTALL_PATH}/sbin" \
    "${NETDATA_INSTALL_PATH}/usr/bin" \
    "${NETDATA_INSTALL_PATH}/usr/sbin" \
    "${NETDATA_INSTALL_PATH}/usr/local"


# -----------------------------------------------------------------------------
# create the makeself archive

run sed "s|NETDATA_VERSION|${FILE_VERSION}|g" <"${NETDATA_MAKESELF_PATH}/makeself.lsm" >"${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp"

run "${NETDATA_MAKESELF_PATH}/makeself.sh" \
    --gzip \
    --complevel 9 \
    --notemp \
    --needroot \
    --target "${NETDATA_INSTALL_PATH}" \
    --header "${NETDATA_MAKESELF_PATH}/makeself-header.sh" \
    --lsm "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp" \
    --license "${NETDATA_MAKESELF_PATH}/makeself-license.txt" \
    --help-header "${NETDATA_MAKESELF_PATH}/makeself-help-header.txt" \
    "${NETDATA_INSTALL_PATH}" \
    "${NETDATA_INSTALL_PATH}.gz.run" \
    "netdata, the real-time performance and health monitoring system" \
    ./system/post-installer.sh \
    ${NULL}

run rm "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp"

# -----------------------------------------------------------------------------
# copy it to the netdata build dir

FILE="netdata-${FILE_VERSION}.gz.run"

run cp "${NETDATA_INSTALL_PATH}.gz.run" "${FILE}"
echo >&2 "Self-extracting installer copied to '${FILE}'"

[ -f netdata-latest.gz.run ] && rm netdata-latest.gz.run
run ln -s "${FILE}" netdata-latest.gz.run
echo >&2 "Self-extracting installer linked to 'netdata-latest.gz.run'"
