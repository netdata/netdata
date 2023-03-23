<!--
title: "StackPulse agent alert notifications"
description: "Send alerts to your StackPulse Netdata integration any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "StackPulse"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/stackpulse/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# StackPulse agent alert notifications

[StackPulse](https://stackpulse.com/) is a software-as-a-service platform for site reliability engineering.
It helps SREs, DevOps Engineers and Software Developers reduce toil and alert fatigue while improving reliability of 
software services by managing, analyzing and automating incident response activities.

Sending Netdata alarm notifications to StackPulse allows you to create smart automated response workflows 
(StackPulse playbooks) that will help you drive down your MTTD and MTTR by performing any of the following:

-   Enriching the incident with data from multiple sources
-   Performing triage actions and analyzing their results
-   Orchestrating incident management and notification flows
-   Performing automatic and semi-automatic remediation actions
-   Analyzing incident data and remediation patterns to improve reliability of your services

To send the notification you need:

1.  Create a Netdata integration in the `StackPulse Administration Portal`, and copy the `Endpoint` URL.

![Creating a Netdata integration in StackPulse](https://user-images.githubusercontent.com/49162938/93023348-d9455a80-f5dd-11ea-8e05-67d07dce93e4.png)

2.  On your node, navigate to `/etc/netdata/` and run the following command:

```sh
$ ./edit-config health_alarm_notify.conf
```

3.  Set the `STACKPULSE_WEBHOOK` variable to `Endpoint` URL you copied earlier:

```
SEND_STACKPULSE="YES"
STACKPULSE_WEBHOOK="https://hooks.stackpulse.io/v1/webhooks/YOUR_UNIQUE_ID"
```

4.  Now restart Netdata using `sudo systemctl restart netdata`, or the [appropriate
    method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system. When your node creates an alarm, you can see the
    associated notification on your StackPulse Administration Portal 

## React to alarms with playbooks

StackPulse allow users to create `Playbooks` giving additional information about events that happen in specific 
scenarios. For example, you could create a Playbook that responds to a "low disk space" alarm by compressing and 
cleaning up storage partitions with dynamic data.

![image](https://user-images.githubusercontent.com/49162938/93207961-4c201400-f74b-11ea-94d1-42a29d007b62.png)
 
![The StackPulse Administration Portal with a Netdata
alarm](https://user-images.githubusercontent.com/49162938/93208199-bfc22100-f74b-11ea-83c4-728be23dcf4d.png) 
### Create Playbooks for Netdata alarms

To create a Playbook, you need to access the StackPulse Administration Portal. After the initial setup, you need to
access the **TRIGGER** tab to define the scenarios used to trigger the event. The following variables are available:

-  `Hostname`: The host that generated the event.
-  `Chart`: The name of the chart.
-  `OldValue` : The previous value of the alarm.
-  `Value`: The current value of the alarm.
-  `Units` : The units of the value.
-  `OldStatus` : The previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL.
-  `State`: The current alarm  status, the acceptable values are the same of `OldStatus`.
-  `Alarm` : The name of the alarm, as given in Netdata's health.d entries.
-  `Date` : The timestamp this event occurred.
-  `Duration` : The duration in seconds of the previous alarm state.
-  `NonClearDuration` : The total duration in seconds this is/was non-clear.
-  `Description` : A short description of the alarm copied from the alarm definition.
-  `CalcExpression` : The expression that was evaluated to trigger the alarm.
-  `CalcParamValues` : The values of the parameters in the expression, at the time of the evaluation.
-  `TotalWarnings` : Total number of alarms in WARNING state.
-  `TotalCritical` : Total number of alarms in CRITICAL state.
-  `ID` : The unique id of the alarm that generated this event.

For more details how to create a scenario, take a look at the [StackPulse documentation](https://docs.stackpulse.io).


