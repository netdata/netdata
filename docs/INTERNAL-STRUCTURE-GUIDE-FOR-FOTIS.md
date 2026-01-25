# Internal Guide for Fotis: Map CSV - Where Integrations Go

This document shows how content will APPEAR on learn.netdata.cloud sidebar.

---

## EXPECTED SIDEBAR STRUCTURE

```
Alerts & Notifications
├── Notifications
│   ├── Agent Dispatched Notifications
│   │   ├── alerta
│   │   ├── aws-sns
│   │   ├── custom
│   │   ├── discord
│   │   ├── dynatrace
│   │   ├── email
│   │   ├── flock
│   │   ├── gotify
│   │   ├── ilert
│   │   ├── irc
│   │   ├── kavenegar
│   │   ├── matrix
│   │   ├── microsoft-teams
│   │   ├── messagebird
│   │   ├── ntfy
│   │   ├── opsgenie
│   │   ├── pagerduty
│   │   ├── prowl
│   │   ├── pushbullet
│   │   ├── pushover
│   │   ├── rocketchat
│   │   ├── signal4
│   │   ├── slack
│   │   ├── smseagle
│   │   ├── sms
│   │   ├── syslog
│   │   ├── telegram
│   │   └── twilio
│   │
│   └── Centralized Cloud Notifications
│       ├── amazon-sns
│       ├── discord
│       ├── ilert
│       ├── mattermost
│       ├── microsoft-teams
│       ├── netdata-mobile-app
│       ├── opsgenie
│       ├── pagerduty
│       ├── rocketchat
│       ├── servicenow
│       ├── slack
│       ├── splunk
│       ├── splunk-victorops
│       ├── telegram
│       └── webhook
│
├── Understanding Alerts
├── Creating and Managing Alerts
├── Alert Configuration Syntax
├── Controlling Alerts and Noise
├── Receiving Notifications
├── Alert Examples and Common Patterns
├── Troubleshooting Alert Behavior
├── Essential Alert Patterns
├── APIs for Alerts and Events
├── Netdata Cloud Alert and Events Features
├── Best Practices for Alerting
└── Alerts and Notifications Architecture
```

---

## WHERE TO ADD ENTRIES IN map.csv

### Line 161: After "Agent Notifications Reference" (line 160)

Add 28 entries for Agent integrations BELOW line 160.

### Line 165: After "Manage alert notification silencing rules" (line 164)

Add 15 entries for Cloud integrations BELOW line 164.

---

## INTEGRATIONS LIST (COPY-PASTE REFERENCE)

### Agent-Dispatched (28)
alerta, aws-sns, custom, discord, dynatrace, email, flock, gotify, ilert, irc, kavenegar, matrix, microsoft-teams, messagebird, ntfy, opsgenie, pagerduty, prowl, pushbullet, pushover, rocketchat, signal4, slack, smseagle, sms, syslog, telegram, twilio

### Cloud-Dispatched (15)
amazon-sns, discord, ilert, mattermost, microsoft-teams, netdata-mobile-app, opsgenie, pagerduty, rocketchat, servicenow, slack, splunk, splunk-victorops, telegram, webhook

---

## NOTHING ELSE TO DO

Just add the entries at the two specified lines. Do not worry about folder structure - it's already correct in learn/.