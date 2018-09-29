#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0+

. $(dirname "${0}")/functions.sh

export LC_ALL=C
umask 002

# Be nice on production environments
renice 19 $$ >/dev/null 2>/dev/null

# -----------------------------------------------------------------------------

STARTIT=1

while [ ! -z "${1}" ]
do
    if [ "${1}" = "--dont-start-it" ]
    then
        STARTIT=0
    else
        echo >&2 "Unknown option '${1}'. Ignoring it."
    fi
    shift
done

deleted_stock_configs=0
if [ ! -f "etc/netdata/.installer-cleanup-of-stock-configs-done" ]
then

    # -----------------------------------------------------------------------------
    progress "Deleting stock configuration files from user configuration directory"

    declare -A configs_signatures=()
    source "system/configs.signatures"

    if [ ! -d etc/netdata ]
        then
        run mkdir -p etc/netdata
    fi

    md5sum="$(which md5sum 2>/dev/null || command -v md5sum 2>/dev/null || command -v md5 2>/dev/null)"
    for x in $(find etc -type f)
    do
        # find it relative filename
        f="${x/etc\/netdata\//}"

        # find the stock filename
        t="${f/.conf.old/.conf}"
        t="${t/.conf.orig/.conf}"

        if [ ! -z "${md5sum}" ]
            then
            # find the checksum of the existing file
            md5="$( ${md5sum} <"${x}" | cut -d ' ' -f 1)"
            #echo >&2 "md5: ${md5}"

            # check if it matches
            if [ "${configs_signatures[${md5}]}" = "${t}" ]
                then
                # it matches the default
                run rm -f "${x}"
                deleted_stock_configs=$(( deleted_stock_configs + 1 ))
            fi
        fi
    done

    touch "etc/netdata/.installer-cleanup-of-stock-configs-done"
fi

# -----------------------------------------------------------------------------
progress "Add user netdata to required user groups"

NETDATA_USER="root"
NETDATA_GROUP="root"
add_netdata_user_and_group "/opt/netdata"
if [ $? -eq 0 ]
    then
    NETDATA_USER="netdata"
    NETDATA_GROUP="netdata"
else
    run_failed "Failed to add netdata user and group"
fi


# -----------------------------------------------------------------------------
progress "Check SSL certificates paths"

if [ ! -f "/etc/ssl/certs/ca-certificates.crt" ]
then
    if [ ! -f /opt/netdata/.curlrc ]
    then
        cacert=

        # CentOS
        [ -f "/etc/ssl/certs/ca-bundle.crt" ] && cacert="/etc/ssl/certs/ca-bundle.crt"

        if [ ! -z "${cacert}" ]
        then
            echo "Creating /opt/netdata/.curlrc with cacert=${cacert}"
            echo >/opt/netdata/.curlrc "cacert=${cacert}"
        else
            run_failed "Failed to find /etc/ssl/certs/ca-certificates.crt"
        fi
    fi
fi


# -----------------------------------------------------------------------------
progress "Install logrotate configuration for netdata"

install_netdata_logrotate || run_failed "Cannot install logrotate file for netdata."


# -----------------------------------------------------------------------------
progress "Install netdata at system init"

install_netdata_service || run_failed "Cannot install netdata init service."


# -----------------------------------------------------------------------------
progress "creating quick links"

dir_should_be_link() {
    local p="${1}" t="${2}" d="${3}" old

    old="${PWD}"
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

dir_should_be_link etc/netdata ../../usr/lib/netdata/conf.d orig

if [ ${deleted_stock_configs} -gt 0 ]
then
    dir_should_be_link etc/netdata ../../usr/lib/netdata/conf.d "000.-.USE.THE.orig.LINK.TO.COPY.AND.EDIT.STOCK.CONFIG.FILES"
fi


# -----------------------------------------------------------------------------

progress "create user config directories"

for x in "python.d" "charts.d" "node.d" "health.d" "statsd.d"
do
    if [ ! -d "etc/netdata/${x}" ]
        then
        run mkdir -p "etc/netdata/${x}" || exit 1
    fi
done


# -----------------------------------------------------------------------------
progress "fix permissions"

run chmod g+rx,o+rx /opt
run chown -R ${NETDATA_USER}:${NETDATA_GROUP} /opt/netdata


# -----------------------------------------------------------------------------

progress "fix plugin permissions"

for x in apps.plugin freeipmi.plugin cgroup-network
do
    f="usr/libexec/netdata/plugins.d/${x}"

    if [ -f "${f}" ]
        then
        run chown root:${NETDATA_GROUP} "${f}"
        run chmod 4750 "${f}"
    fi
done

# fix the fping binary
if [ -f bin/fping ]
then
    run chown root:${NETDATA_GROUP} bin/fping
    run chmod 4750 bin/fping
fi


# -----------------------------------------------------------------------------

if [ ${STARTIT} -eq 1 ]
then
    progress "starting netdata"

    restart_netdata "/opt/netdata/bin/netdata"
    if [ $? -eq 0 ]
        then
        download_netdata_conf "${NETDATA_USER}:${NETDATA_GROUP}" "/opt/netdata/etc/netdata/netdata.conf" "http://localhost:19999/netdata.conf"
        netdata_banner "is installed and running now!"
    else
        generate_netdata_conf "${NETDATA_USER}:${NETDATA_GROUP}" "/opt/netdata/etc/netdata/netdata.conf" "http://localhost:19999/netdata.conf"
        netdata_banner "is installed now!"
    fi
else
    generate_netdata_conf "${NETDATA_USER}:${NETDATA_GROUP}" "/opt/netdata/etc/netdata/netdata.conf" "http://localhost:19999/netdata.conf"
    netdata_banner "is installed now!"
fi
