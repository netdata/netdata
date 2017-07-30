#!/usr/bin/env bash

. $(dirname "${0}")/functions.sh

export LC_ALL=C
umask 002

# Be nice on production environments
renice 19 $$ >/dev/null 2>/dev/null


# -----------------------------------------------------------------------------
progress "Checking new configuration files"

declare -A configs_signatures=()
. system/configs.signatures

if [ ! -d etc/netdata ]
    then
    run mkdir -p etc/netdata
fi

md5sum="$(which md5sum 2>/dev/null || command -v md5sum 2>/dev/null || command -v md5 2>/dev/null)"
for x in $(find etc.new -type f)
do
    # find it relative filename
    f="${x/etc.new\/netdata\//}"
    t="${x/etc.new\//etc\/}"
    d=$(dirname "${t}")

    #echo >&2 "x: ${x}"
    #echo >&2 "t: ${t}"
    #echo >&2 "d: ${d}"

    if [ ! -d "${d}" ]
        then
        run mkdir -p "${d}"
    fi

    if [ ! -f "${t}" ]
        then
        run cp "${x}" "${t}"
        continue
    fi

    if [ ! -z "${md5sum}" ]
        then
        # find the checksum of the existing file
        md5="$(cat "${t}" | ${md5sum} | cut -d ' ' -f 1)"
        #echo >&2 "md5: ${md5}"

        # check if it matches
        if [ "${configs_signatures[${md5}]}" = "${f}" ]
            then
            run cp "${x}" "${t}"
        fi
    fi
    
    if ! [[ "${x}" =~ .*\.orig ]]
        then
        run mv "${x}" "${t}.orig"
    fi
done

run rm -rf etc.new


# -----------------------------------------------------------------------------
progress "Add user netdata to required user groups"

NETDATA_USER="root"
NETDATA_GROUP="root"
add_netdata_user_and_group
if [ $? -eq 0 ]
    then
    NETDATA_USER="netdata"
    NETDATA_GROUP="netdata"
else
    run_failed "Failed to add netdata user and group"
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


# -----------------------------------------------------------------------------
progress "starting netdata"

restart_netdata "/opt/netdata/bin/netdata"
if [ $? -eq 0 ]
    then
    netdata_banner "is installed and running now!"
else
    netdata_banner "is installed now!"
fi
