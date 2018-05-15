#!/usr/bin/env bash

# basic version of netdata notifier to work with bash3
# only mail and syslog destinations are supported, one recipient each
# - email: DEFAULT_RECIPIENT_EMAIL, "root" by default
# - syslog: "netdata" with local6 facility; disabled by default
# - also: setting recipient to "disabled" or "silent" stops notifications for this alert

# in /etc/netdata/health_alarm_notify.conf set something like
# EMAIL_SENDER="netdata@gesdev-vm.m0.maxidom.ru"
# SEND_EMAIL="YES"
# DEFAULT_RECIPIENT_EMAIL="root"
# SEND_SYSLOG="YES"
# SYSLOG_FACILITY="local6"
# DEFAULT_RECIPIENT_SYSLOG="netdata"

# netdata
# real-time performance and health monitoring, done right!
# (C) 2017 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#
# Script to send alarm notifications for netdata
#
# Supported notification methods:
#  - emails by @ktsaou
#  - syslog messages by @Ferroin
#  - all the rest is pruned :)

# -----------------------------------------------------------------------------
# testing notifications

if [ \( "${1}" = "test" -o "${2}" = "test" \) -a "${#}" -le 2 ]
then
    if [ "${2}" = "test" ]
    then
        recipient="${1}"
    else
        recipient="${2}"
    fi

    [ -z "${recipient}" ] && recipient="sysadmin"

    id=1
    last="CLEAR"
    test_res=0
    for x in "WARNING" "CRITICAL"  "CLEAR"
    do
        echo >&2
        echo >&2 "# SENDING TEST ${x} ALARM TO ROLE: ${recipient}"

        "${0}" "${recipient}" "$(hostname)" 1 1 "${id}" "$(date +%s)" "test_alarm" "test.chart" "test.family" "${x}" "${last}" 100 90 "${0}" 1 $((0 + id)) "units" "this is a test alarm to verify notifications work" "new value" "old value"
        if [ $? -ne 0 ]
        then
            echo >&2 "# FAILED"
            test_res=1
        else
            echo >&2 "# OK"
        fi

        last="${x}"
        id=$((id + 1))
    done

    exit $test_res
fi

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"
export LC_ALL=C

# -----------------------------------------------------------------------------

PROGRAM_NAME="$(basename "${0}")"

logdate() {
    date "+%Y-%m-%d %H:%M:%S"
}

log() {
    local status="${1}"
    shift

    echo >&2 "$(logdate): ${PROGRAM_NAME}: ${status}: ${*}"

}

warning() {
    log WARNING "${@}"
}

error() {
    log ERROR "${@}"
}

info() {
    log INFO "${@}"
}

fatal() {
    log FATAL "${@}"
    exit 1
}

debug=${NETDATA_ALARM_NOTIFY_DEBUG-0}
debug() {
    [ "${debug}" = "1" ] && log DEBUG "${@}"
}

docurl() {
    if [ -z "${curl}" ]
        then
        error "\${curl} is unset."
        return 1
    fi

    if [ "${debug}" = "1" ]
        then
        echo >&2 "--- BEGIN curl command ---"
        printf >&2 "%q " ${curl} "${@}"
        echo >&2
        echo >&2 "--- END curl command ---"

        local out=$(mktemp /tmp/netdata-health-alarm-notify-XXXXXXXX)
        local code=$(${curl} ${curl_options} --write-out %{http_code} --output "${out}" --silent --show-error "${@}")
        local ret=$?
        echo >&2 "--- BEGIN received response ---"
        cat >&2 "${out}"
        echo >&2
        echo >&2 "--- END received response ---"
        echo >&2 "RECEIVED HTTP RESPONSE CODE: ${code}"
        rm "${out}"
        echo "${code}"
        return ${ret}
    fi

    ${curl} ${curl_options} --write-out %{http_code} --output /dev/null --silent --show-error "${@}"
    return $?
}

# -----------------------------------------------------------------------------
# this is to be overwritten by the config file

custom_sender() {
    info "not sending custom notification for ${status} of '${host}.${chart}.${name}'"
}


# -----------------------------------------------------------------------------

# check for BASH v4+ (required for associative arrays)
[ $(( ${BASH_VERSINFO[0]} )) -lt 3 ] && \
    fatal "BASH version 3 or later is required (this is ${BASH_VERSION})."

# -----------------------------------------------------------------------------
# defaults to allow running this script by hand

[ -z "${NETDATA_CONFIG_DIR}"   ] && NETDATA_CONFIG_DIR="$(dirname "${0}")/../../../../etc/netdata"
[ -z "${NETDATA_CACHE_DIR}"    ] && NETDATA_CACHE_DIR="$(dirname "${0}")/../../../../var/cache/netdata"
[ -z "${NETDATA_REGISTRY_URL}" ] && NETDATA_REGISTRY_URL="https://registry.my-netdata.io"

# -----------------------------------------------------------------------------
# parse command line parameters

roles="${1}"               # the roles that should be notified for this event
host="${2}"                # the host generated this event
unique_id="${3}"           # the unique id of this event
alarm_id="${4}"            # the unique id of the alarm that generated this event
event_id="${5}"            # the incremental id of the event, for this alarm id
when="${6}"                # the timestamp this event occurred
name="${7}"                # the name of the alarm, as given in netdata health.d entries
chart="${8}"               # the name of the chart (type.id)
family="${9}"              # the family of the chart
status="${10}"             # the current status : REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
old_status="${11}"         # the previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
value="${12}"              # the current value of the alarm
old_value="${13}"          # the previous value of the alarm
src="${14}"                # the line number and file the alarm has been configured
duration="${15}"           # the duration in seconds of the previous alarm state
non_clear_duration="${16}" # the total duration in seconds this is/was non-clear
units="${17}"              # the units of the value
info="${18}"               # a short description of the alarm
value_string="${19}"       # friendly value (with units)
old_value_string="${20}"   # friendly old value (with units)

# -----------------------------------------------------------------------------
# find a suitable hostname to use, if netdata did not supply a hostname

this_host=$(hostname -s 2>/dev/null)
[ -z "${host}" ] && host="${this_host}"

# -----------------------------------------------------------------------------
# screen statuses we don't need to send a notification

# don't do anything if this is not WARNING, CRITICAL or CLEAR
if [ "${status}" != "WARNING" -a "${status}" != "CRITICAL" -a "${status}" != "CLEAR" ]
then
    info "not sending notification for ${status} of '${host}.${chart}.${name}'"
    exit 1
fi

# don't do anything if this is CLEAR, but it was not WARNING or CRITICAL
if [ "${old_status}" != "WARNING" -a "${old_status}" != "CRITICAL" -a "${status}" = "CLEAR" ]
then
    info "not sending notification for ${status} of '${host}.${chart}.${name}' (last status was ${old_status})"
    exit 1
fi

# -----------------------------------------------------------------------------
# load configuration

# By default fetch images from the global public registry.
# This is required by default, since all notification methods need to download
# images via the Internet, and private registries might not be reachable.
# This can be overwritten at the configuration file.
images_base_url="https://registry.my-netdata.io"

# curl options to use
curl_options=

# needed commands
# if empty they will be searched in the system path
curl=
sendmail=

# enable / disable features
SEND_EMAIL="YES"
SEND_SYSLOG="YES"

# syslog configs
SYSLOG_FACILITY="local6"

# email configs
EMAIL_SENDER=
DEFAULT_RECIPIENT_EMAIL="root"
EMAIL_CHARSET=$(locale charmap 2>/dev/null)

# load the user configuration
# this will overwrite the variables above
if [ -f "${NETDATA_CONFIG_DIR}/health_alarm_notify.conf" ]
    then
    source "${NETDATA_CONFIG_DIR}/health_alarm_notify.conf"
else
    error "Cannot find file ${NETDATA_CONFIG_DIR}/health_alarm_notify.conf. Using internal defaults."
fi

# If we didn't autodetect the character set for e-mail and it wasn't
# set by the user, we need to set it to a reasonable default.  UTF-8
# should be correct for almost all modern UNIX systems.
if [ -z ${EMAIL_CHARSET} ]
    then
    EMAIL_CHARSET="UTF-8"
fi

# disable if role = silent or disabled
if [[ "${role}" = "silent" || "${role}" = "disabled" ]]; then
    SEND_EMAIL="NO"
    SEND_SYSLOG="NO"
fi

if [[ "SEND_EMAIL" != "NO" ]]; then
    to_email=$DEFAULT_RECIPIENT_EMAIL
else
    to_email=''
fi

if [[ "SEND_SYSLOG" != "NO" ]]; then
    to_syslog=$DEFAULT_RECIPIENT_SYSLOG
else
    to_syslog=''
fi

# if we need sendmail, check for the sendmail command
if [ "${SEND_EMAIL}" = "YES" -a -z "${sendmail}" ]
    then
    sendmail="$(which sendmail 2>/dev/null || command -v sendmail 2>/dev/null)"
    if [ -z "${sendmail}" ]
        then
        debug "Cannot find sendmail command in the system path. Disabling email notifications."
        SEND_EMAIL="NO"
    fi
fi

# if we need logger, check for the logger command
if [ "${SEND_SYSLOG}" = "YES" -a -z "${logger}" ]
    then
    logger="$(which logger 2>/dev/null || command -v logger 2>/dev/null)"
    if [ -z "${logger}" ]
        then
        debug "Cannot find logger command in the system path. Disabling syslog notifications."
        SEND_SYSLOG="NO"
    fi
fi

# check that we have at least a method enabled
if [   "${SEND_EMAIL}"          != "YES" \
    -a "${SEND_SYSLOG}"         != "YES" \
    ]
    then
    fatal "All notification methods are disabled. Not sending notification for host '${host}', chart '${chart}' to '${roles}' for '${name}' = '${value}' for status '${status}'."
fi

# -----------------------------------------------------------------------------
# get the date the alarm happened

date=$(date --date=@${when} "${DATE_FORMAT}" 2>/dev/null)
[ -z "${date}" ] && date=$(date "${DATE_FORMAT}" 2>/dev/null)
[ -z "${date}" ] && date=$(date --date=@${when} 2>/dev/null)
[ -z "${date}" ] && date=$(date 2>/dev/null)

# -----------------------------------------------------------------------------
# function to URL encode a string

urlencode() {
    local string="${1}" strlen encoded pos c o

    strlen=${#string}
    for (( pos=0 ; pos<strlen ; pos++ ))
    do
        c=${string:${pos}:1}
        case "${c}" in
            [-_.~a-zA-Z0-9])
                o="${c}"
                ;;

            *)
                printf -v o '%%%02x' "'${c}"
                ;;
        esac
        encoded+="${o}"
    done

    REPLY="${encoded}"
    echo "${REPLY}"
}

# -----------------------------------------------------------------------------
# function to convert a duration in seconds, to a human readable duration
# using DAYS, MINUTES, SECONDS

duration4human() {
    local s="${1}" d=0 h=0 m=0 ds="day" hs="hour" ms="minute" ss="second" ret
    d=$(( s / 86400 ))
    s=$(( s - (d * 86400) ))
    h=$(( s / 3600 ))
    s=$(( s - (h * 3600) ))
    m=$(( s / 60 ))
    s=$(( s - (m * 60) ))

    if [ ${d} -gt 0 ]
    then
        [ ${m} -ge 30 ] && h=$(( h + 1 ))
        [ ${d} -gt 1 ] && ds="days"
        [ ${h} -gt 1 ] && hs="hours"
        if [ ${h} -gt 0 ]
        then
            ret="${d} ${ds} and ${h} ${hs}"
        else
            ret="${d} ${ds}"
        fi
    elif [ ${h} -gt 0 ]
    then
        [ ${s} -ge 30 ] && m=$(( m + 1 ))
        [ ${h} -gt 1 ] && hs="hours"
        [ ${m} -gt 1 ] && ms="minutes"
        if [ ${m} -gt 0 ]
        then
            ret="${h} ${hs} and ${m} ${ms}"
        else
            ret="${h} ${hs}"
        fi
    elif [ ${m} -gt 0 ]
    then
        [ ${m} -gt 1 ] && ms="minutes"
        [ ${s} -gt 1 ] && ss="seconds"
        if [ ${s} -gt 0 ]
        then
            ret="${m} ${ms} and ${s} ${ss}"
        else
            ret="${m} ${ms}"
        fi
    else
        [ ${s} -gt 1 ] && ss="seconds"
        ret="${s} ${ss}"
    fi

    REPLY="${ret}"
    echo "${REPLY}"
}

# -----------------------------------------------------------------------------
# email sender

send_email() {
    local ret= opts=
    if [[ "${SEND_EMAIL}" == "YES" ]]
        then

        if [[ ! -z "${EMAIL_SENDER}" ]]
            then
            if [[ "${EMAIL_SENDER}" =~ \".*\"\ \<.*\> ]]
                then
                # the name includes single quotes
                opts=" -f $(echo "${EMAIL_SENDER}" | cut -d '<' -f 2 | cut -d '>' -f 1) -F $(echo "${EMAIL_SENDER}" | cut -d '<' -f 1)"
            elif [[ "${EMAIL_SENDER}" =~ \'.*\'\ \<.*\> ]]
                then
                # the name includes double quotes
                opts=" -f $(echo "${EMAIL_SENDER}" | cut -d '<' -f 2 | cut -d '>' -f 1) -F $(echo "${EMAIL_SENDER}" | cut -d '<' -f 1)"
            elif [[ "${EMAIL_SENDER}" =~ .*\ \<.*\> ]]
                then
                # the name does not have any quotes
                opts=" -f $(echo "${EMAIL_SENDER}" | cut -d '<' -f 2 | cut -d '>' -f 1) -F '$(echo "${EMAIL_SENDER}" | cut -d '<' -f 1)'"
            else
                # no name at all
                opts=" -f ${EMAIL_SENDER}"
            fi
        fi

        if [[ "${debug}" = "1" ]]
            then
            echo >&2 "--- BEGIN sendmail command ---"
            printf >&2 "%q " "${sendmail}" -t ${opts}
            echo >&2
            echo >&2 "--- END sendmail command ---"
        fi

        "${sendmail}" -t ${opts}
        ret=$?

        if [ ${ret} -eq 0 ]
        then
            info "sent email notification for: ${host} ${chart}.${name} is ${status} to '${to_email}'"
            return 0
        else
            error "failed to send email notification for: ${host} ${chart}.${name} is ${status} to '${to_email}' with error code ${ret}."
            return 1
        fi
    fi

    return 1
}

# -----------------------------------------------------------------------------
# syslog sender

send_syslog() {
    local facility=${SYSLOG_FACILITY:-"local6"} level='info' targets="${1}"
    local priority='' message='' host='' port='' prefix=''
    local temp1='' temp2=''

    [[ "${SEND_SYSLOG}" == "YES" ]] || return 1

    if [[ "${status}" == "CRITICAL" ]] ; then
        level='crit'
    elif [[ "${status}" == "WARNING" ]] ; then
        level='warning'
    fi

    for target in ${targets} ; do
        priority="${facility}.${level}"
        message=''
        host=''
        port=''
        prefix=''
        temp1=''
        temp2=''

        prefix=$(echo ${target} | cut -d '/' -f 2)
        temp1=$(echo ${target} | cut -d '/' -f 1)

        if [[ ${prefix} != ${temp1} ]] ; then
            if (echo ${temp1} | grep -q '@' ) ; then
                temp2=$(echo ${temp1} | cut -d '@' -f 1)
                host=$(echo ${temp1} | cut -d '@' -f 2)

                if [ ${temp2} != ${host} ] ; then
                    priority=${temp2}
                fi

                port=$(echo ${host} | rev | cut -d ':' -f 1 | rev)

                if ( echo ${host} | grep -E -q '\[.*\]' ) ; then
                    if ( echo ${port} | grep -q ']' ) ; then
                        port=''
                    else
                        host=$(echo ${host} | rev | cut -d ':' -f 2- | rev)
                    fi
                else
                    if [ ${port} = ${host} ] ; then
                        port=''
                    else
                        host=$(echo ${host} | cut -d ':' -f 1)
                    fi
                fi
            else
                priority=${temp1}
            fi
        fi

        # message="${prefix} ${status} on ${host} at ${date}: ${chart} ${value_string}"

        message="${prefix} ${status}: ${chart} ${value_string}"

        if [ ${host} ] ; then
            logger_options="${logger_options} -n ${host}"
            if [ ${port} ] ; then
                logger_options="${logger_options} -P ${port}"
            fi
        fi

        ${logger} -p ${priority} ${logger_options} "${message}"
    done

    return $?
}


# -----------------------------------------------------------------------------
# prepare the content of the notification

# the url to send the user on click
urlencode "${host}" >/dev/null; url_host="${REPLY}"
urlencode "${chart}" >/dev/null; url_chart="${REPLY}"
urlencode "${family}" >/dev/null; url_family="${REPLY}"
urlencode "${name}" >/dev/null; url_name="${REPLY}"
goto_url="${NETDATA_REGISTRY_URL}/goto-host-from-alarm.html?host=${url_host}&chart=${url_chart}&family=${url_family}&alarm=${url_name}&alarm_unique_id=${unique_id}&alarm_id=${alarm_id}&alarm_event_id=${event_id}"

# the severity of the alarm
severity="${status}"

# the time the alarm was raised
duration4human ${duration} >/dev/null; duration_txt="${REPLY}"
duration4human ${non_clear_duration} >/dev/null; non_clear_duration_txt="${REPLY}"
raised_for="(was ${old_status} for ${duration_txt})"

# the key status message
status_message="status unknown"

# the color of the alarm
color="grey"

# the alarm value
alarm="${name//_/ } = ${value_string}"

# the image of the alarm
image="${images_base_url}/images/seo-performance-128.png"

# prepare the title based on status
case "${status}" in
	CRITICAL)
        image="${images_base_url}/images/alert-128-red.png"
        status_message="is critical"
        color="#ca414b"
        ;;

    WARNING)
        image="${images_base_url}/images/alert-128-orange.png"
        status_message="needs attention"
        color="#ffc107"
		;;

	CLEAR)
        image="${images_base_url}/images/check-mark-2-128-green.png"
    	status_message="recovered"
		color="#77ca6d"
		;;
esac

if [ "${status}" = "CLEAR" ]
then
    severity="Recovered from ${old_status}"
    if [ ${non_clear_duration} -gt ${duration} ]
    then
        raised_for="(alarm was raised for ${non_clear_duration_txt})"
    fi

    # don't show the value when the status is CLEAR
    # for certain alarms, this value might not have any meaning
    alarm="${name//_/ } ${raised_for}"

elif [ "${old_status}" = "WARNING" -a "${status}" = "CRITICAL" ]
then
    severity="Escalated to ${status}"
    if [ ${non_clear_duration} -gt ${duration} ]
    then
        raised_for="(alarm is raised for ${non_clear_duration_txt})"
    fi

elif [ "${old_status}" = "CRITICAL" -a "${status}" = "WARNING" ]
then
    severity="Demoted to ${status}"
    if [ ${non_clear_duration} -gt ${duration} ]
    then
        raised_for="(alarm is raised for ${non_clear_duration_txt})"
    fi

else
    raised_for=
fi

# prepare HTML versions of elements
info_html=
[ ! -z "${info}" ] && info_html=" <small><br/>${info}</small>"

raised_for_html=
[ ! -z "${raised_for}" ] && raised_for_html="<br/><small>${raised_for}</small>"


# -----------------------------------------------------------------------------
# send the syslog message

send_syslog ${to_syslog}

SENT_SYSLOG=$?

# -----------------------------------------------------------------------------
# send the email

send_email <<EOF
To: ${to_email}
Subject: ${host} ${status_message} - ${name//_/ } - ${chart}
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="multipart-boundary"

This is a MIME-encoded multipart message

--multipart-boundary
Content-Type: text/plain; encoding=${EMAIL_CHARSET}
Content-Disposition: inline
Content-Transfer-Encoding: 8bit

${host} ${status_message}

${alarm} ${info}
${raised_for}

Chart   : ${chart}
Family  : ${family}
Severity: ${severity}
URL     : ${goto_url}
Source  : ${src}
Date    : ${date}
Value   : ${value_string}
Notification generated on ${this_host}

--multipart-boundary
Content-Type: text/html; encoding=${EMAIL_CHARSET}
Content-Disposition: inline
Content-Transfer-Encoding: 8bit

<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; box-sizing: border-box; font-size: 14px; margin: 0; padding: 0;">
<body style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 14px; width: 100% !important; min-height: 100%; line-height: 1.6; background: #f6f6f6; margin:0; padding: 0;">
<table>
    <tbody>
    <tr>
        <td style="vertical-align: top;" valign="top"></td>
        <td width="700" style="vertical-align: top; display: block !important; max-width: 700px !important; clear: both !important; margin: 0 auto; padding: 0;" valign="top">
            <div style="max-width: 700px; display: block; margin: 0 auto; padding: 20px;">
                <table width="100%" cellpadding="0" cellspacing="0" style="background: #fff; border: 1px solid #e9e9e9;">
                    <tbody>
                    <tr>
                        <td bgcolor="#eee" style="padding: 5px 20px 5px 20px; background-color: #eee;">
                            <div style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 20px; color: #777; font-weight: bold;">netdata notification</div>
                        </td>
                    </tr>
                    <tr>
                        <td bgcolor="${color}" style="font-size: 16px; vertical-align: top; font-weight: 400; text-align: center; margin: 0; padding: 10px; color: #ffffff; background: ${color} !important; border: 1px solid ${color}; border-top-color: ${color};" align="center" valign="top">
                            <h1 style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-weight: 400; margin: 0;">${host} ${status_message}</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="vertical-align: top;" valign="top">
                            <div style="margin: 0; padding: 20px; max-width: 700px;">
                                <table width="100%" cellpadding="0" cellspacing="0" style="max-width:700px">
                                    <tbody>
                                    <tr>
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding:0 0 20px;" align="left" valign="top">
                                            <span>${chart}</span>
                                            <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Chart</span>
                                        </td>
                                    </tr>
                                    <tr>
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;" align="left" valign="top">
                                            <span>${value_string}</span>
                                            <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Value</span>
                                        </td>
                                    </tr>
                                    <tr style="margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;" align="left" valign="top">
                                            <span><b>${alarm}</b>${info_html}</span>
                                            <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Alarm</span>
                                        </td>
                                    </tr>
                                    <tr>
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;" align="left" valign="top">
                                            <span>${family}</span>
                                            <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Family</span>
                                        </td>
                                    </tr>
                                    <tr style="margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;" align="left" valign="top">
                                            <span>${severity}</span>
                                            <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Severity</span>
                                        </td>
                                    </tr>
                                    <tr style="margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;" align="left" valign="top"><span>${date}</span>
                                            <span>${raised_for_html}</span> <span style="display: block; color: #666666; font-size: 12px; font-weight: 300; line-height: 1; text-transform: uppercase;">Time</span>
                                        </td>
                                    </tr>
                                    <tr style="margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 18px; vertical-align: top; margin: 0; padding: 0 0 20px;">
                                            <a href="${goto_url}" style="font-size: 14px; color: #ffffff; text-decoration: none; line-height: 1.5; font-weight: bold; text-align: center; display: inline-block; text-transform: capitalize; background: #35568d; border-width: 1px; border-style: solid; border-color: #2b4c86; margin: 0; padding: 10px 15px;" target="_blank">View Netdata</a>
                                        </td>
                                    </tr>
                                    <tr style="text-align: center; margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 11px; vertical-align: top; margin: 0; padding: 10px 0 0 0; color: #666666;" align="center" valign="bottom">The source of this alarm is line <code>${src}</code><br/>(alarms are configurable, edit this file to adapt the alarm to your needs)
                                        </td>
                                    </tr>
                                    <tr style="text-align: center; margin: 0; padding: 0;">
                                        <td style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; font-size: 12px; vertical-align: top; margin:0; padding: 20px 0 0 0; color: #666666; border-top: 1px solid #f0f0f0;" align="center" valign="bottom">Sent by
                                            <a href="https://mynetdata.io/" target="_blank">netdata</a>, the real-time performance and health monitoring, on <code>${this_host}</code>.
                                        </td>
                                    </tr>
                                    </tbody>
                                </table>
                            </div>
                        </td>
                    </tr>
                    </tbody>
                </table>
            </div>
        </td>
    </tr>
    </tbody>
</table>
</body>
</html>
--multipart-boundary--
EOF

SENT_EMAIL=$?

# -----------------------------------------------------------------------------
# let netdata know

if [   ${SENT_EMAIL}        -eq 0 \
    -o ${SENT_SYSLOG}       -eq 0 \
    ]
    then
    # we did send something
    exit 0
fi

# we did not send anything
exit 1
