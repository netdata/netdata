#!/usr/bin/env bash

. $(dirname "${0}")/functions.sh

# -----------------------------------------------------------------------------
progress "Checking new configuration files"

declare -A configs_signatures=()
. system/configs.signatures

if [ ! -d etc/netdata ]
    then
    mkdir -p etc/netdata
fi

for x in $(find etc/netdata.new -name '*.conf' -type f)
do
    # find it relative filename
    f="${x/*etc\/netdata.new\//}"
    t="${x/*etc\/netdata.new\/etc\/netdata/}"
    d=$(dirname "${d}")

    if [ ! -d "${d}" ]
        then
        run mkdir -p "${d}"
    fi

    if [ ! -f "${t}" ]
        then
        run cp "${x}" "${t}"
        continue
    fi

    # find the checksum of the existing file
    md5="$(cat "${t}" | ${md5sum} | cut -d ' ' -f 1)"
    echo >&2 "debug md5 of '${t}': ${md5}"

    # check if it matches
    if [ "${configs_signatures[${md5}]}" = "${f}" ]
        then
        run cp "${x}" "${t}"
    else
        run cp "${x}" "${t}.orig"
    fi
done


# -----------------------------------------------------------------------------
progress "Add user netdata to required user groups"

NETDATA_USER=root
NETDATA_GROUP=root
NETDATA_ADDED_TO_DOCKER=0
NETDATA_ADDED_TO_NGINX=0
NETDATA_ADDED_TO_VARNISH=0
NETDATA_ADDED_TO_HAPROXY=0
NETDATA_ADDED_TO_ADM=0
NETDATA_ADDED_TO_NSD=0
if [ ${UID} -eq 0 ]
    then
    portable_add_group netdata && NETDATA_USER=netdata
    portable_add_user  netdata && NETDATA_GROUP=netdata
    portable_add_user_to_group docker   netdata && NETDATA_ADDED_TO_DOCKER=1
    portable_add_user_to_group nginx    netdata && NETDATA_ADDED_TO_NGINX=1
    portable_add_user_to_group varnish  netdata && NETDATA_ADDED_TO_VARNISH=1
    portable_add_user_to_group haproxy  netdata && NETDATA_ADDED_TO_HAPROXY=1
    portable_add_user_to_group adm      netdata && NETDATA_ADDED_TO_ADM=1
    portable_add_user_to_group nsd      netdata && NETDATA_ADDED_TO_NSD=1
    run_ok
else
    run_failed "The installer does not run as root."
fi


# -----------------------------------------------------------------------------
progress "Install logrotate configuration for netdata"

if [ ${UID} -eq 0 ]
    then
    if [ -d /etc/logrotate.d ]
        then
        if [ ! -f /etc/logrotate.d/netdata ]
            then
            run cp system/netdata.logrotate /etc/logrotate.d/netdata
        fi

        if [ -f /etc/logrotate.d/netdata ]
            then
            run chmod 644 /etc/logrotate.d/netdata
        fi
    else
       run_failed "logrotate dir /etc/logrotate.d is not available."
    fi
else
    run_failed "The installer does not run as root."
fi


# -----------------------------------------------------------------------------
progress "Install netdata at system init"

if [ "${UID}" -eq 0 ]
    then
    install_netdata_service
else
    run_failed "The installer does not run as root."
fi


# -----------------------------------------------------------------------------
progress "creating quick links"

dir_should_be_link() {
    local p="${1}" t="${2}" d="${3}" old

    old="$(pwd)"
    cd "${p}" || return 0

    if [ -e "${d}" ]
        then
        if [ -h "${d}" ]
            then
            run rm "${d}"
        else
            run mv -f "${d}" "${d}.old.$$"
        fi
    fi

    run ln -s "${t}" "${d}"
    cd "${old}"
}

dir_should_be_link .   bin    sbin
dir_should_be_link usr ../bin bin
dir_should_be_link usr ../bin sbin
dir_should_be_link usr .      local

dir_should_be_link . etc/netdata           netdata-configs
dir_should_be_link . usr/share/netdata/web netdata-web-files
dir_should_be_link . usr/libexec/netdata   netdata-plugins
dir_should_be_link . var/lib/netdata       netdata-dbs
dir_should_be_link . var/cache/netdata     netdata-metrics
dir_should_be_link . var/log/netdata       netdata-logs


# -----------------------------------------------------------------------------
progress "fix permissions"

for x in etc/netdata var/log/netdata var/lib/netdata var/cache/netdata usr/share/netdata
do
    if [ -d "${x}" ]
        then
        run chown -R ${NETDATA_USER}:${NETDATA_GROUP} ${x}
    fi
done


# -----------------------------------------------------------------------------
progress "fix plugin permissions"

for x in apps.plugin freeipmi.plugin
do
    f="usr/libexec/netdata/plugins.d/${x}"

    if [ -f "${f}" ]
        then
        run chown root:${NETDATA_GROUP} "${f}"
        run chmod 4750 "${d}"
    fi
done


# -----------------------------------------------------------------------------
netdata_banner "is installed now!"
