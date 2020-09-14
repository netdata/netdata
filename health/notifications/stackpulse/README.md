<!--
title: "Send notifications to Stackpulse"
description: "Send alerts to your Stackpulse Netdata integration any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "Opsgenie"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/opsgenie/README.md
-->

# Stackpulse

StackPulse is a Software-as-a-Service platform for Site Reliablility Engineering. It helps SREs, DevOps Engineers and 
Software Developers reduce toil and alert fatigue while improving reliability of software services by managing, 
analyzing and automating incident response activities.

# Send notifications to Stackpulse

Sending Netdata alarm notifications to StackPulse allows you to create smart automated response workflows 
(StackPulse playbooks) that will help you drive down your MTTD and MTTR by performing any of the following:

* Enriching the incident with data from multiple sources
* Performing triage actions and analyzing their results
* Orchestrating incident management and notification flows
* Performing automatic and semi-automatic remediation actions
* Analzying incident data and remediation patterns to improve reliability of your services

To send the notification you need :

1.  Create a Netdata integration in the `StackPulse Administration Portal` and copy the `Endpoint` URL.

![image](https://user-images.githubusercontent.com/49162938/93023348-d9455a80-f5dd-11ea-8e05-67d07dce93e4.png)

2.  Go to `/etc/netdata/` and run the following commands:

```sh
$ ./edit-config health_alarm_notify.conf
```

3.  Set the `Stackpulse` variable with the `Endpoint` copied:

```
SEND_STACKPULSE="YES"
STACKPULSE_WEBHOOK="https://hooks.stackpulse.io/v1/webhooks/YOUR_UNIQUE_ID"
```

4.  Now [restart Netdata](/docs/getting-started.md#start-stop-and-restart-netdata)

when an alarm happens, you will have the following notification on your StackPulse Administration Portal

![image](https://user-images.githubusercontent.com/49162938/93023407-5244b200-f5de-11ea-84fe-4f85ad1d1ba1.png)

# Organizing alarms with Playbooks

Stackpulse allow users to create `Playbooks` giving additional information about events that happen in specific 
scenarios, for example, we created three different playbooks, the first has only `Warning` alarms, the
 second has o`Critical` alarms and finally we have a playbook with all alarms.
 
## Creating Playbook for Netdata alarms

To create a `Playbook` you need to access the `StackPulse Administration Portal`. After to do the initial setup, it 
is necessary to access the `TRIGGER` tab to define the scenarios used to trigger the event, the following variables are
available:

-  `Hostname`: The host that generated the event.
-  `Chart`: The name of the chart.
-  `OldValue` : The previous value of the alarm.
-  `Value`: The current value of the alarm
-  `Units` : The units of the value.
-  `OldStatus` : The previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL
-  `State`: The current alarm  status, the acceptable values are the same of `OldStatus`.
-  `Alarm` : The name of the alarm, as given in Netdata health.d entries
-  `Date` : The timestamp this event occurred.
-  `Duration` : The duration in seconds of the previous alarm state.
-  `NonClearDuration` : The total duration in seconds this is/was non-clear
-  `Description` : A short description of the alarm copied from the alarm definition.
-  `CalcExpression` : The expression that was evaluated to trigger the alarm
-  `CalcParamValues` : The values of the parameters in the expression, at the time of the evaluation
-  `TotalWarnings` : Total number of alarms in WARNING state
-  `TotalCritical` : Total number of alarms in CRITICAL state
-  `ID` : The unique id of the alarm that generated this event

For more details how to create a scenario, take a look at [Stackpulse documentation](https://docs.stackpulse.io).



[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fopsgenie%2FREADME%2FDonations-netdata-has-received&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)