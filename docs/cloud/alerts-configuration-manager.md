# Creating Alerts with Netdata Alerts Configuration Manager

The Netdata Alerts Configuration Manager enables users with [Business subscriptions](https://www.netdata.cloud/pricing/) to create alerts from the Netdata Dashboard with an intuitive user interface. 

## Using Alerts Configuration Manager

- Go to the `Metrics` tab and navigate to the `chart` you want to alert on.

- Click the `Alert icon` on the top right corner of the chart.
![Alert Icon](https://github.com/netdata/netdata/assets/96257330/88bb4e86-cbc7-4e01-9c84-6b901188c0de)

- Alert Configuration Manager will open up with the `default` thresholds. Modify the configuration as required and the alert definition on the right will be updated dynamically.
![Alert Configuration Modal](https://github.com/netdata/netdata/assets/96257330/ce39ae64-2ffe-4576-8c92-b7918bb8c91c)

- If you want more fine grained control or access to more advanced settings, enable `Show advanced` 
![Advance Options](https://github.com/netdata/netdata/assets/96257330/b409b31b-6dc7-484c-a2a4-4e5e471d029b)

- Copy the alert definition that is generated in the code box and add it to an existing [health configuration file](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#edit-health-configuration-files) or a new custom file under `<path to netdata install>/etc/netdata/health.d/` on a `Parent Agent` or a `Standalone Child Agent`.
![Copy the Alert Configuration](https://github.com/netdata/netdata/assets/96257330/c948e280-c6c8-426f-98b1-2b5256cc2707)

- Reload Netdata Alert Health checks `<path to netdata install>/usr/sbin/netdatacli reload-health` and the new alert is now configured.


## Alerts Configuration Manager Sections

1. **Alert Name, Description and Summary Section**
![Alert Name, Description and Summary Section](https://github.com/netdata/netdata/assets/96257330/50680344-ccd9-439d-80f7-7f26f217a842)

    - **Alert Template Name**: This field uniquely identifies an alert and corresponds to the [template](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-alarm-or-template) field of the Alert configuration. The Alerts Configuration Manager provides a default name for an Alert template but we recommend you to modify this to have a meaningful name for your configured alert.
    - **Alert Template Description**: This field provides a description to the alert and corresponds to the [info](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-info) line of the Alert configuration.
    - **Alert Summary**: This field enables the users to customise a title for the Alert Notifications (via [Notification integrations](https://learn.netdata.cloud/docs/alerting/notifications/centralized-cloud-notifications)) and corresponds to the [summary](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-summary) line of the Alert configuration.

2. **Alert Type**
![Alert Type](https://github.com/netdata/netdata/assets/96257330/c8d83a65-90e7-4b03-9279-585abb359662)
    The users can select the type of Alert that they want to create:
    - Standard `threshold` based alerts.
    - `Variance` based alerts
    - `Anomalies` based alerts


3. **Metrics Lookup, Filtering and Formula Section**
![Metrics Lookup, Filtering and Formula Section](https://github.com/netdata/netdata/assets/96257330/784c3f54-d7ce-45ea-9505-3f789d6d3ddb)

    - **Metrics Lookup**: This field defines the parameters of a database lookup that is needed to compute the value that will be compared against the alert definition and corresponds to the `[lookup](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-lookup)` line of the Alert configuration. The Alerts Configuration Manager provides a default selection for the lookup and can be modified to suit your requirements. The parameters that you are allowed to modify are: 
        - METHOD `(avg, sum, min, max, cv, stddev)`
        - COMPUTATION `(sum of all dimensions or individually for each dimension)`
        - DIMENSIONS `(All dimensions, or a selection of dimensions)` 
        - DURATION `(the period in time to run the lookup)` and 
        - OPTIONS `(absolute, unaligned, percentage, min2max)`. This field is available in the `Basic Options`.

    - **Alert Filtering**: This field enables the users to filter the alert health checks to be run only for specific components of the infrastructure and helps in fine grained configuration of the alerts. This is only available in the `Advance Options`.
        - `HOSTS / NODES` - By default all hosts are selected. You can choose / enter a wildcard matching a list of hosts you want this alert health check to run on. This field corresponds to the [hosts](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-hosts) line of the Alert configuration.
        - `INSTANCES` - By default all instances are selected. You can choose / enter a wildcard matching a list of instances you want this alert health check to run on. This field corresponds to the [charts](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-charts) line of the Alert configuration.
        - `CHART LABELS` - By default all chart labels are selected. You can choose a chart label and select or enter a wildcard matching a list of chart label values you want this alert health check to run on. This field corresponds to the [chart labels](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-chart-labels) line of the Alert configuration.
        - `OS` - By default all Operating Systems are selected. You can choose which OSes are relevant for this alert health check to run on. This field corresponds to the [os](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-os) line of the Alert configuration.

    - **Formula / Calculation**: This field enables the user to define a formula that will be run on top of the `Lookup` above. The result of the lookup is available in `$this` and after defining the formula, the result of the `formula` is also stored in `$this` and can be accessed while setting the alert thresholds. This field corresponds to the [calc](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-calc) line of the Alert configuration. This is only available in the `Advance Options`.

4. **Alert Thresholds**
![Alert Thresholds](https://github.com/netdata/netdata/assets/96257330/1545d22d-c729-46f5-84cd-f82654d2cb12)
    - **Warning and Critical Threshold**: This field enables the users to set a threshold to raise alerts as  `Warning` or `Critical` and corresponds to the [warn](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-lines-warn-and-crit) line of the Alert configuration.
    - **Recovery Thresholds**: This field enables the users to set a different threshold to de-escalate the severity of an alert from `Critical to Warning` or `Warning to Clear`.
    - **Delay Notifications**: This field enables the users to set a delay on notifications for an alert severity `escalation` or `de-escalation` and corresponds to the [delay](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-delay) line of the Alert configuration.
    - **Check Interval**: This fiels enables the users to define the frequency of the health check for the alert being defined and corresponds to the [every](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-every) line of the Alert configuration.

5. **Agent Specific Options**
    These options are only available on the `Netdata Agent` and not honoured on `Netdata Cloud`.
![Agent Specific Options](https://github.com/netdata/netdata/assets/96257330/d2bab429-1e2e-40d0-a892-79ea83bb5f25)
    - **Repeat Notifications**: This field allows the users to define if you want to repeat sending the `alert notifications` when the alert is active and corresponds to the [repeat](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-repeat) line of the Alert configuration.
    - **Send to**: This field enables the users to define a `user role` to which the alert notifications need to be sent or `silence` the notifications and corresponds to the [to](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-to) line of the Alert configuration.
    - **Custom Exec Script**: This field enables the users to define a custom script to be executed when the alert is triggered (but needs to be carefully designed as it neads to call the `health_alarm_notify.sh` module) and corresponds to the [exec](https://learn.netdata.cloud/docs/alerting/health-configuration-reference#alert-line-exec) line of the Alert Configuration.
