#!/usr/bin/env bash

# (C) Costa Tsaousis
# pushover support by Jan Arnold

me="${0}"

# -----------------------------------------------------------------------------
# parse command line parameters

recipient="${1}"   # the recepient of the email
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

# -----------------------------------------------------------------------------
# screen statuses we don't need to send a notification

# don't do anything if this is not WARNING, CRITICAL or CLEAR
if [ "${status}" != "WARNING" -a "${status}" != "CRITICAL" -a "${status}" != "CLEAR" ]
then
    echo >&2 "${me}: not sending notification for ${status} on '${chart}.${name}'"
    exit 1
fi

# don't do anything if this is CLEAR, but it was not WARNING or CRITICAL
if [ "${old_status}" != "WARNING" -a "${old_status}" != "CRITICAL" -a "${status}" = "CLEAR" ]
then
    echo >&2 "${me}: not sending notification for ${status} on '${chart}.${name}' (last status was ${old_status})"
    exit 1
fi

# -----------------------------------------------------------------------------
# load configuration

# needed commands
# if empty they will be searched in the system path
curl=
sendmail=

# enable / disable features
SEND_EMAIL="YES"
SEND_PUSHOVER="YES"

# pushover configs
PUSHOVER_APP_TOKEN=
DEFAULT_RECIPIENT_PUSHOVER=
declare -A role_recipients_pushover=()

# email configs
DEFAULT_RECIPIENT_EMAIL="root"
declare -A role_recipients_email=()

if [ -f "${NETDATA_CONFIG_DIR}/health_alarm_notify.conf" ]
    then
    source "${NETDATA_CONFIG_DIR}/health_alarm_notify.conf"
fi

# -----------------------------------------------------------------------------
# find the exact recipient per method

to_email="${role_recipients_email[${recipient}]}"
[ -z "${to_email}" ] && to_email="${DEFAULT_RECIPIENT_EMAIL}"
[ -z "${to_email}" ] && to_email="root"

to_pushover="${role_recipients_pushover[${recipient}]}"
[ -z "${to_pushover}" ] && to_pushover="${DEFAULT_RECIPIENT_EMAIL}"
[ -z "${to_pushover}" ] && SEND_PUSHOVER="NO"

# -----------------------------------------------------------------------------
# verify the delivery methods supported

[ -z "${PUSHOVER_APP_TOKEN}" ] && SEND_PUSHOVER="NO"

if [ "${SEND_PUSHOVER}" = "YES" -a -z "${curl}" ]
    then
    curl="$(which curl 2>/dev/null || command -v curl 2>/dev/null)"
    [ -z "${curl}" ] && SEND_PUSHOVER="NO"
fi

if [ "${SEND_EMAIL}" = "YES" -a -z "${sendmail}" ]
    then
    sendmail="$(which sendmail 2>/dev/null || command -v sendmail 2>/dev/null)"
    [ -z "${sendmail}" ] && SEND_EMAIL="NO"
fi

# check that we have at least a method enabled
if [ "${SEND_EMAIL}" != "YES" -a "${SEND_PUSHOVER}" != "YES" ]
    then
    echo >&2 "I don't have a means to send a notification. Sorry!"
    exit 1
fi

# -----------------------------------------------------------------------------
# get the system hostname

[ -z "${hostname}" ] && hostname="${NETDATA_HOSTNAME}"
[ -z "${hostname}" ] && hostname="${NETDATA_REGISTRY_HOSTNAME}"
[ -z "${hostname}" ] && hostname="$(hostname 2>/dev/null)"

# -----------------------------------------------------------------------------
# get the date the alarm happened

date="$(date --date=@${when} 2>/dev/null)"
[ -z "${date}" ] && date="$(date 2>/dev/null)"

# -----------------------------------------------------------------------------
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

# -----------------------------------------------------------------------------
# email sender

send_email() {
    if [ "${SEND_EMAIL}" = "YES" ]
        then

        "${sendmail}" -t

        if [ $? -eq 0 ]
        then
            echo >&2 "${me}: Sent notification email for ${status} on '${chart}.${name}'"
            return 0
        else
            echo >&2 "${me}: FAILED to send notification email for ${status} on '${chart}.${name}'"
            return 1
        fi
    fi

    return 1
}

# -----------------------------------------------------------------------------
# pushover sender

send_pushover() {
    local apptoken="${1}" usertoken="${2}" title="${3}" message="${4}" httpcode

    if [ "${SEND_PUSHOVER}" = "YES" -a ! -z "${apptoken}" -a ! -z "${usertoken}" -a ! -z "${title}" -a ! -z "${message}" ]
        then

        httpcode=$(${curl} --write-out %{http_code} --silent --output /dev/null \
            --form-string "token=${apptoken}" \
            --form-string "user=${usertoken}" \
            --form-string "html=1" \
            --form-string "title=${title}" \
            --form-string "message=${message}" \
            https://api.pushover.net/1/messages.json)

        if [ "${httpcode}" == "200" ]
        then
            echo >&2 "${me}: Sent notification push for ${status} on '${chart}.${name}'"
            return 0
        else
            echo >&2 "${me}: FAILED to send notification push for ${status} on '${chart}.${name}' with HTTP error code ${httpcode}."
            return 1
        fi
    fi

    return 1
}


# -----------------------------------------------------------------------------
# prepare the content of the notification

# description of the alarm
[ ! -z "${info}" ] && info=" <small><br/>${info}</small>"

# the url to send the user on click
goto_url="${NETDATA_REGISTRY_URL}/goto-host-from-alarm.html?machine_guid=${NETDATA_REGISTRY_UNIQUE_ID}&chart=${chart}&family=${family}"

# the severity of the alarm
severity="${status}"

# the time the alarm was raised
raised_for="<br/><small>(was ${old_status,,} for $(duration4human ${duration}))</small>"

# the key status message
status_message="status unknown"

# the color of the alarm
color="grey"

# the alarm value
alarm="${name//_/ } = ${value} ${units}"

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

if [ "${status}" = "CLEAR" ]
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


# -----------------------------------------------------------------------------
# send the pushover

send_pushover "${PUSHOVER_APP_TOKEN}" "${to_pushover}" "${hostname} ${status_message} - ${chart}.${name}" "<font size="5" color=\"${color}\"><b>${hostname} ${status_message}</b></font>

<b>${alarm}</b>${info}

Chart: ${chart}
Family: ${family}
Severity: ${severity}
Time: ${date}
${raised_for}
<a href=\"${goto_url}\">View Netdata</a>

<small>The source of this alarm is line ${src}</small>"

SENT_PUSHOVER=$?

# -----------------------------------------------------------------------------
# send the email

cat <<EOF | send_email
To: ${to_email}
Subject: ${hostname} ${status_message} - ${chart}.${name}
Content-Type: text/html

<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" style="font-family: 'Helvetica Neue', 'Helvetica', Helvetica, Arial, sans-serif; box-sizing: border-box; font-size: 14px; margin: 0; padding: 0;">
<body style="font-family:'Helvetica Neue','Helvetica',Helvetica,Arial,sans-serif;font-size:14px;width:100%!important;min-height:100%;line-height:1.6;background:#f6f6f6;margin:0;padding:0">
<table>
    <tbody>
    <tr>
        <td style="vertical-align:top;" valign="top"></td>
        <td width="700" style="vertical-align:top;display:block!important;max-width:700px!important;clear:both!important;margin:0 auto;padding:0" valign="top">
            <div style="max-width:700px;display:block;margin:0 auto;padding:20px">
                <table width="100%" cellpadding="0" cellspacing="0"
                       style="background:#fff;border:1px solid #e9e9e9">
                    <tbody>
                    <tr>
                        <td bgcolor="#eee"
                            style="padding: 5px 20px 5px 20px;background-color:#eee;">
                            <div style="font-size:20px;color:#777;font-weight: bold;">netdata notification</div>
                        </td>
                    </tr>
                    <tr>
                        <td bgcolor="${color}"
                            style="font-size:16px;vertical-align:top;font-weight:400;text-align:center;margin:0;padding:10px;color:#ffffff;background:${color}!important;border:1px solid ${color};border-top-color:${color}" align="center" valign="top">
                            <h1 style="font-weight:400;margin:0">${hostname} ${status_message}</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="vertical-align:top" valign="top">
                            <div style="margin:0;padding:20px;max-width:700px">
                                <table width="100%" cellpadding="0" cellspacing="0" style="max-width:700px">
                                    <tbody>
                                    <tr>
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top">
                                            <span>${chart}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Chart</span>
                                        </td>
                                    </tr>
                                    <tr style="margin:0;padding:0">
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top">
                                            <span><b>${alarm}</b>${info}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Alarm</span>
                                        </td>
                                    </tr>
                                    <tr>
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top">
                                            <span>${family}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Family</span>
                                        </td>
                                    </tr>
                                    <tr style="margin:0;padding:0">
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top">
                                            <span>${severity}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Severity</span>
                                        </td>
                                    </tr>
                                    <tr style="margin:0;padding:0">
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top"><span>${date}</span>
                                            <span>${raised_for}</span> <span
                                                    style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Time</span>
                                        </td>
                                    </tr>
                                    <!--
                                    <tr style="margin:0;padding:0">
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px">
                                            <a href="${goto_url}" style="font-size:14px;color:#ffffff;text-decoration:none;line-height:1.5;font-weight:bold;text-align:center;display:inline-block;text-transform:capitalize;background:#35568d;border-width:1px;border-style:solid;border-color:#2b4c86;margin:0;padding:10px 15px" target="_blank">View Netdata</a>
                                        </td>
                                    </tr>
                                    -->
                                    <tr style="text-align:center;margin:0;padding:0">
                                        <td style="font-size:11px;vertical-align:top;margin:0;padding:10px 0 0 0;color:#666666"
                                            align="center" valign="bottom">The source of this alarm is line <code>${src}</code>
                                        </td>
                                    </tr>
                                    <tr style="text-align:center;margin:0;padding:0">
                                        <td style="font-size:12px;vertical-align:top;margin:0;padding:20px 0 0 0;color:#666666;border-top:1px solid #f0f0f0"
                                            align="center" valign="bottom">Sent by
                                            <a href="https://mynetdata.io/" target="_blank">netdata</a>, the real-time performance monitoring.
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
EOF

SENT_EMAIL=$?

# -----------------------------------------------------------------------------
# let netdata know

# we did send somehting
[ ${SENT_EMAIL} -eq 0 -o ${SENT_PUSHOVER} -eq 0 ] && exit 0

# we did not send anything
exit 1
