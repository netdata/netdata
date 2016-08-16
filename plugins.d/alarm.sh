#!/usr/bin/env bash

type="${1}"       # WARNING or CRITICAL
name="${2}"       # the name of the alarm, as given in netdata health.d entries
chart="${3}"      # the name of the chart (type.id)
status="${4}"     # the current status
old_status="${5}" # the previous status
value="${6}"      # the current value
old_value="${7}"  # the previous value
src="${8}"        # the line number and file the alarm has been configured
duration="${9}"   # the duration in seconds the previous state took

# don't do anything if this is not RAISED or OFF
[ "${status}" != "RAISED" -a "${status}" != "OFF" ] && exit 0

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

# prepare the title
raised_for=""
case "${status}" in
	RAISED)
		if [ "${type}" = "CRITICAL" ]
		then
			# CRITICAL - red
			status_message="is critical"
			color="#ca414b"
		else
			# WARNING - yellow
			status_message="needs attention"
			color="#caca4b"
		fi
		;;

	OFF)
		if [ "${type}" = "CRITICAL" ]
		then
			# CRITICAL
			status_message="recovered"
		else
			# WARNING
			status_message="back to normal"
		fi
		color="#77ca6d"
		if [ "${old_status}" = "RAISED" ]
		then
			raised_for="<br/>(was in ${type,,} state for $(duration4human ${duration}))"
		fi
		;;

	*)      status_message="status unknown"
		color="grey"
		;;
esac

# send the email
cat <<EOF | /usr/sbin/sendmail -t
To: root
Subject: ${type} ${hostname} ${status_message} - ${chart}.${name}
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
                        <td bgcolor="#555555"
                            style="padding:5px;background-color:#555555;background:repeating-linear-gradient(-45deg,#aaa,#aaa 14px,#ccc 14px,#ccc 24px)!important">
                            <div style="font-size:20px;color: #444;text-decoration: none;font-weight: bold;">netdata alert</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="font-size:16px;vertical-align:top;font-weight:400;text-align:center;margin:0;padding:10px;color:#ffffff;background:${color}!important;border:1px solid ${color};border-top-color:${color}" align="center" valign="top">
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
                                            <span>${name} = ${value}</span>
                                            <span style="display:block;color:#666666;font-size:12px;font-weight:300;line-height:1;text-transform:uppercase">Alarm</span>
                                        </td>
                                    </tr>
                                    <tr style="margin:0;padding:0">
                                        <td style="font-size:18px;vertical-align:top;margin:0;padding:0 0 20px"
                                            align="left" valign="top">
                                            <span>${type}</span>
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
