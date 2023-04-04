# Custom Agent alert notifications

Netdata Agent's alert notification feature allows you to send custom notifications to any endpoint you choose.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

## Prerequisites

You need to have terminal access to the Agent you wish to configure.

## Configure Netdata to send alert notifications to a custom endpoint

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_CUSTOM` to `YES`.
2. The `DEFAULT_RECIPIENT_CUSTOM`'s value is dependent on how you handle the `${to}` variable inside the `custom_sender()` function.  
   All roles will default to this variable if left unconfigured.
3. Edit the `custom_sender()` function.  
   You can look at the other senders in `/usr/libexec/netdata/plugins.d/alarm-notify.sh` for examples of how to modify the function in this configuration file.

    The following is a sample `custom_sender()` function in `health_alarm_notify.conf`, to send an SMS via an imaginary HTTPS endpoint to the SMS gateway:

    ```sh
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

    The supported variables that you can use for the function's `msg` variable are:

    | Variable name               | Description                                                                      |
    |:---------------------------:|:---------------------------------------------------------------------------------|
    | `${alarm}`                  | Like "name = value units"                                                        |
    | `${status_message}`         | Like "needs attention", "recovered", "is critical"                               |
    | `${severity}`               | Like "Escalated to CRITICAL", "Recovered from WARNING"                           |
    | `${raised_for}`             | Like "(alarm was raised for 10 minutes)"                                         |
    | `${host}`                   | The host generated this event                                                    |
    | `${url_host}`               | Same as ${host} but URL encoded                                                  |
    | `${unique_id}`              | The unique id of this event                                                      |
    | `${alarm_id}`               | The unique id of the alarm that generated this event                             |
    | `${event_id}`               | The incremental id of the event, for this alarm id                               |
    | `${when}`                   | The timestamp this event occurred                                                |
    | `${name}`                   | The name of the alarm, as given in netdata health.d entries                      |
    | `${url_name}`               | Same as ${name} but URL encoded                                                  |
    | `${chart}`                  | The name of the chart (type.id)                                                  |
    | `${url_chart}`              | Same as ${chart} but URL encoded                                                 |
    | `${family}`                 | The family of the chart                                                          |
    | `${url_family}`             | Same as ${family} but URL encoded                                                |
    | `${status}`                 | The current status : REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL |
    | `${old_status}`             | The previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL |
    | `${value}`                  | The current value of the alarm                                                   |
    | `${old_value}`              | The previous value of the alarm                                                  |
    | `${src}`                    | The line number and file the alarm has been configured                           |
    | `${duration}`               | The duration in seconds of the previous alarm state                              |
    | `${duration_txt}`           | Same as ${duration} for humans                                                   |
    | `${non_clear_duration}`     | The total duration in seconds this is/was non-clear                              |
    | `${non_clear_duration_txt}` | Same as ${non_clear_duration} for humans                                         |
    | `${units}`                  | The units of the value                                                           |
    | `${info}`                   | A short description of the alarm                                                 |
    | `${value_string}`           | Friendly value (with units)                                                      |
    | `${old_value_string}`       | Friendly old value (with units)                                                  |
    | `${image}`                  | The URL of an image to represent the status of the alarm                         |
    | `${color}`                  | A color in  AABBCC format for the alarm                                          |
    | `${goto_url}`               | The URL the user can click to see the netdata dashboard                          |
    | `${calc_expression}`        | The expression evaluated to provide the value for the alarm                      |
    | `${calc_param_values}`      | The value of the variables in the evaluated expression                           |
    | `${total_warnings}`         | The total number of alarms in WARNING state on the host                          |
    | `${total_critical}`         | The total number of alarms in CRITICAL state on the host                         |

You can then have different `${to}` variables per **role**, by editing `DEFAULT_RECIPIENT_CUSTOM` with the variable you want, in the following entries at the bottom of the same file:

```conf
role_recipients_custom[sysadmin]="systems"
role_recipients_custom[domainadmin]="domains"
role_recipients_custom[dba]="databases systems"
role_recipients_custom[webmaster]="marketing development"
role_recipients_custom[proxyadmin]="proxy-admin"
role_recipients_custom[sitemgr]="sites"
```

An example working configuration would be:

```conf
#------------------------------------------------------------------------------
# custom notifications

SEND_CUSTOM="YES"
DEFAULT_RECIPIENT_CUSTOM=""

# The custom_sender() is a custom function to do whatever you need to do
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

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
