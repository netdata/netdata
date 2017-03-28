#!/usr/bin/env bash

. $(dirname "${0}")/functions.sh

progress "Add user netdata to required user groups"

NETDATA_ADDED_TO_DOCKER=0
NETDATA_ADDED_TO_NGINX=0
NETDATA_ADDED_TO_VARNISH=0
NETDATA_ADDED_TO_HAPROXY=0
NETDATA_ADDED_TO_ADM=0
NETDATA_ADDED_TO_NSD=0
if [ ${UID} -eq 0 ]
    then
    portable_add_group netdata
    portable_add_user netdata
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
    if [ -d /etc/logrotate.d -a ! -f /etc/logrotate.d/netdata ]
        then
        run cp system/netdata.logrotate /etc/logrotate.d/netdata
    fi

    if [ -f /etc/logrotate.d/netdata ]
        then
        run chmod 644 /etc/logrotate.d/netdata
    fi
fi

# -----------------------------------------------------------------------------
progress "Install netdata at system init"

if [ "${UID}" -eq 0 ]
    then
    install_netdata_service
fi


