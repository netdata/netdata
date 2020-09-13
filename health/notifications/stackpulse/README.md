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

1.  Create a Netdata integration in the StackPulse Administration Portal and copy the `Endpoint` URL.
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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fopsgenie%2FREADME%2FDonations-netdata-has-received&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)