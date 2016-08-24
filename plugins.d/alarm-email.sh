#!/usr/bin/env bash

me="${0}"

sendmail="$(which sendmail 2>/dev/null || command -v sendmail 2>/dev/null)"
if [ -z "${sendmail}" ]
then
    echo >&2 "I cannot send emails - there is no sendmail command available."
fi

sendmail_from_pipe() {
    "${sendmail}" -t

    if [ $? -eq 0 ]
    then
        echo >&2 "${me}: Sent notification email for ${status} on '${chart}.${name}'"
        return 0
    else
        echo >&2 "${me}: FAILED to send notification email for ${status} on '${chart}.${name}'"
        return 1
    fi
}

name="${1}"       # the name of the alarm, as given in netdata health.d entries
chart="${2}"      # the name of the chart (type.id)
status="${3}"     # the current status : UNITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
old_status="${4}" # the previous status: UNITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
value="${5}"      # the current value
old_value="${6}"  # the previous value
src="${7}"        # the line number and file the alarm has been configured
duration="${8}"   # the duration in seconds the previous state took
non_clear_duration="${9}" # the total duration in seconds this is non-clear

# get the system hostname
hostname="$(hostname)"

# get the current date
date="$(date)"

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
raised_for="<br/>(was ${old_status,,} for $(duration4human ${duration}))"
status_message="status unknown"
color="grey"
alarm="${name} = ${value}"

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
    echo >&2 "${me}: not sending notification email for ${status} on '${chart}.${name}'"
    exit 0
elif [ "${old_status}" != "WARNING" -a "${old_status}" != "CRITICAL" -a "${status}" = "CLEAR" ]
then
    # don't do anything if this is CLEAR, but it was not WARNING or CRITICAL
    echo >&2 "${me}: not sending notification email for ${status} on '${chart}.${name}' (last status was ${old_status})"
    exit 0
elif [ "${status}" = "CLEAR" ]
then
    severity="Recovered from ${old_status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/>(had issues for $(duration4human ${non_clear_duration}))"
    fi

elif [ "${old_status}" = "WARNING" -a "${status}" = "CRITICAL" ]
then
    severity="Escalated to ${status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/>(has issues for $(duration4human ${non_clear_duration}))"
    fi

elif [ "${old_status}" = "CRITICAL" -a "${status}" = "WARNING" ]
then
    severity="Demoted to ${status}"
    if [ $non_clear_duration -gt $duration ]
    then
        raised_for="<br/>(has issues for $(duration4human ${non_clear_duration}))"
    fi

else
    raised_for=
fi

# send the email
cat <<EOF | sendmail_from_pipe
To: root
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
                                            <span>${alarm}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Alarm</span>
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
