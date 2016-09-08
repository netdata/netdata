#!/usr/bin/env bash
# based on alarm-email.sh by netdata
# by Jan Arnold
# v0.1
# 2016-09-12
#
# This is a shell script for using the Pushover (https://pushover.net) push service.
# Copy this script to the netdata plugin folder which by default is "/usr/libexec/netdata/plugins.d/".
# In the config file for the alarm you want to use it for add:
# "exec: /usr/libexec/netdata/plugins.d/alarm-pushover.sh"
# Once Pushover client is installed on your phone or computer, you need to create an account to receive a 
# user token. This is needed so the push reaches your device.
#
# Generate an apptoken at https://pushover.net and insert that and the usertoken into the
# "healt_pushover_tokens.conf" file which has to reside in the netdata config dir which by
# default is "/etc/netdata/".
#
# Further help for pushover: https://pushover.net/faq

me="${0}"

curl="$(which curl 2>/dev/null || command -v curl 2>/dev/null)"
if [ -z "${curl}" ]
then
    echo >&2 "I cannot send push notifications - there is no curl command available."
    exit 1
fi

if [ -f "${NETDATA_CONFIG_DIR}/health_pushover_tokens.conf" ]
    then
    source "${NETDATA_CONFIG_DIR}/health_pushover_tokens.conf"
else
    echo >&2 "${me}: FAILED to source ${NETDATA_CONFIG_DIR}/health_pushover_tokens.conf for push token"
    exit 1
fi

if ! [ "$APPTOKEN" ]
    then
    echo >&2 "${me}: FAILED - Please set pushover APPTOKEN in ${NETDATA_CONFIG_DIR}/health_pushover_tokens.conf"
    exit 1
fi

if ! [ "$USERTOKEN" ]
    then
    echo >&2 "${me}: FAILED - Please set pushover USERTOKEN in ${NETDATA_CONFIG_DIR}/health_pushover_tokens.conf"
    exit 1
fi

curl_pushover() {
    httpcode=$(curl --write-out %{http_code} --silent --output /dev/null \
        --form-string "token=$APPTOKEN" \
        --form-string "user=$USERTOKEN" \
        --form-string "html=1" \
        --form-string "title=$TITLE" \
        --form-string "message=$MESSAGE" \
        https://api.pushover.net/1/messages.json)

    if [ "$httpcode" == "200" ]
    then
        echo >&2 "${me}: Sent notification push for ${status} on '${chart}.${name}'"
        return 0
    else
        echo >&2 "${me}: FAILED to send notification push for ${status} on '${chart}.${name}'"
        return 1
    fi
}

recipient="${1}"   # the recepient of the push
hostname="${2}"    # the hostname this event refers to
unique_id="${3}"   # the unique id of this event
alarm_id="${4}"    # the unique id of the alarm that generated this event
event_id="${5}"    # the incremental id of the event, for this alarm
when="${6}"        # the timestamp this event occured
name="${7}"        # the name of the alarm, as given in netdata health.d entries
chart="${8}"       # the name of the chart (type.id)
family="${9}"      # the family of the chart
status="${10}"     # the current status : UNITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
old_status="${11}" # the previous status: UNITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
value="${12}"      # the current value
old_value="${13}"  # the previous value
src="${14}"        # the line number and file the alarm has been configured
duration="${15}"   # the duration in seconds the previous state took
non_clear_duration="${16}" # the total duration in seconds this is non-clear
units="${17}"      # the units of the value
info="${18}"       # a short description of the alarm

[ ! -z "${info}" ] && info=" <small><br/>${info}</small>"

# get the system hostname
[ -z "${hostname}" ] && hostname="${NETDATA_HOSTNAME}"
[ -z "${hostname}" ] && hostname="${NETDATA_REGISTRY_HOSTNAME}"
[ -z "${hostname}" ] && hostname="$(hostname 2>/dev/null)"

goto_url="${NETDATA_REGISTRY_URL}/goto-host-from-alarm.html?machine_guid=${NETDATA_REGISTRY_UNIQUE_ID}&chart=${chart}&family=${family}"

date="$(date --date=@${when} 2>/dev/null)"
[ -z "${date}" ] && date="$(date 2>/dev/null)"

# convert a duration in seconds, to a human readable duration
# using DAYS, MINUTES, SECONDS
duration4human() {
    local s="${1}" d=0 h=0 m=0 ds="day" hs="hour" ms="minute" ss="second"
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
            echo "${d} ${ds} and ${h} ${hs}"
        else
            echo "${d} ${ds}"
        fi
    elif [ ${h} -gt 0 ]
    then
        [ ${s} -ge 30 ] && m=$(( m + 1 ))
        [ ${h} -gt 1 ] && hs="hours"
        [ ${m} -gt 1 ] && ms="minutes"
        if [ ${m} -gt 0 ]
        then
            echo "${h} ${hs} and ${m} ${ms}"
        else
            echo "${h} ${hs}"
        fi
    elif [ ${m} -gt 0 ]
    then
        [ ${m} -gt 1 ] && ms="minutes"
        [ ${s} -gt 1 ] && ss="seconds"
        if [ ${s} -gt 0 ]
        then
            echo "${m} ${ms} and ${s} ${ss}"
        else
            echo "${m} ${ms}"
        fi
    else
        [ ${s} -gt 1 ] && ss="seconds"
        echo "${s} ${ss}"
    fi
}

severity="${status}"
raised_for="<br/><small>(was ${old_status,,} for $(duration4human ${duration}))</small>"
status_message="status unknown"
color="grey"
alarm="${name} = ${value} ${units}"

# prepare the title based on status
case "${status}" in
	CRITICAL)
        status_message="is critical"
        color="#ca414b"
        ;;

    WARNING)
        status_message="needs attention"
        color="#caca4b"
		;;

	CLEAR)
    	status_message="recovered"
		color="#77ca6d"

		# don't show the value when the status is CLEAR
		# for certain alarms, this value might not have any meaning
		alarm="${name}"
		;;
esac

if [ "${status}" != "WARNING" -a "${status}" != "CRITICAL" -a "${status}" != "CLEAR" ]
then
    # don't do anything if this is not WARNING, CRITICAL or CLEAR
    echo >&2 "${me}: not sending notification push for ${status} on '${chart}.${name}'"
    exit 0
elif [ "${old_status}" != "WARNING" -a "${old_status}" != "CRITICAL" -a "${status}" = "CLEAR" ]
then
    # don't do anything if this is CLEAR, but it was not WARNING or CRITICAL
    echo >&2 "${me}: not sending notification push for ${status} on '${chart}.${name}' (last status was ${old_status})"
    exit 0
elif [ "${status}" = "CLEAR" ]
then
    severity="Recovered from ${old_status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/><small>(had issues for $(duration4human ${non_clear_duration}))</small>"
    fi

elif [ "${old_status}" = "WARNING" -a "${status}" = "CRITICAL" ]
then
    severity="Escalated to ${status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/><small>(has issues for $(duration4human ${non_clear_duration}))</small>"
    fi

elif [ "${old_status}" = "CRITICAL" -a "${status}" = "WARNING" ]
then
    severity="Demoted to ${status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/><small>(has issues for $(duration4human ${non_clear_duration}))</small>"
    fi

else
    raised_for=
fi

# generate push message
TITLE="${hostname} ${status_message} - ${chart}.${name}"
MESSAGE="<font size="5" color=\"${color}\"><b>${hostname} ${status_message}</b></font>

<b>${alarm}</b>${info}

Chart: ${chart}
Family: ${family}
Severity: ${severity}
Time: ${date}
${raised_for}
<a href=\"${goto_url}\">View Netdata</a>

<small>The source of this alarm is line ${src}</small>"

# send the push
curl_pushover
