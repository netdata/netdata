# Custom

Netdata allows you to send custom notifications, to any endpoint you choose.
To configure custom notifications, you will need to define the `custom_sender()` function in `health_alarm_notify.conf` 
You can look at the other senders in `/usr/libexec/netdata/plugins.d/alarm-notify.sh` for examples.
As with other notifications, you will also need to define the recipient list in `DEFAULT_RECIPIENT_CUSTOM` and/or the `role_recipients_custom` array.

The following is a sample `custom_sender` function to send an SMS via an imaginary HTTPS endpoint to the SMS gateway:
```
 custom_sender() {
    # example human readable SMS
    local msg="${host} ${status_message}: ${alarm} ${raised_for}"

    # limit it to 160 characters and encode it for use in a URL
    urlencode "${msg:0:160}" >/dev/null; msg="${REPLY}"

    # a space separated list of the recipients to send alarms to
    to="${1}"

    for phone in ${to}; do
      httpcode=$(docurl -X POST \
				    --data-urlencode "From=XXX" \
				    --data-urlencode "To=${phone}" \
				    --data-urlencode "Body=${msg}" \
				    -u "${accountsid}:${accounttoken}" \
        https://domain.website.com/)

       if [ "${httpcode}" = "200" ]; then
         info "sent custom notification ${msg} to ${phone}"
         sent=$((sent + 1))
       else
         error "failed to send custom notification ${msg} to ${phone} with HTTP error code ${httpcode}."
       fi
    done
}
```

Variables available to the custom_sender:

 - ${to_custom}          the list of recipients for the alarm
 - ${host}               the host generated this event
 - ${url_host}           same as ${host} but URL encoded
 - ${unique_id}          the unique id of this event
 - ${alarm_id}           the unique id of the alarm that generated this event
 - ${event_id}           the incremental id of the event, for this alarm id
 - ${when}               the timestamp this event occurred
 - ${name}               the name of the alarm, as given in netdata health.d entries
 - ${url_name}           same as ${name} but URL encoded
 - ${chart}              the name of the chart (type.id)
 - ${url_chart}          same as ${chart} but URL encoded
 - ${family}             the family of the chart
 - ${url_family}         same as ${family} but URL encoded
 - ${status}             the current status : REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
 - ${old_status}         the previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
 - ${value}              the current value of the alarm
 - ${old_value}          the previous value of the alarm
 - ${src}                the line number and file the alarm has been configured
 - ${duration}           the duration in seconds of the previous alarm state
 - ${duration_txt}       same as ${duration} for humans
 - ${non_clear_duration} the total duration in seconds this is/was non-clear
 - ${non_clear_duration_txt} same as ${non_clear_duration} for humans
 - ${units}              the units of the value
 - ${info}               a short description of the alarm
 - ${value_string}       friendly value (with units)
 - ${old_value_string}   friendly old value (with units)
 - ${image}              the URL of an image to represent the status of the alarm
 - ${color}              a color in #AABBCC format for the alarm
 - ${goto_url}           the URL the user can click to see the netdata dashboard
 - ${calc_expression}    the expression evaluated to provide the value for the alarm
 - ${calc_param_values}  the value of the variables in the evaluated expression
 - ${total_warnings}     the total number of alarms in WARNING state on the host
 - ${total_critical}     the total number of alarms in CRITICAL state on the host

The following are more human friendly:

 - ${alarm}              like "name = value units"
 - ${status_message}     like "needs attention", "recovered", "is critical"
 - ${severity}           like "Escalated to CRITICAL", "Recovered from WARNING"
 - ${raised_for}         like "(alarm was raised for 10 minutes)"

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fcustom%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
