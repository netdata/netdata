# Expected Sidebar Structure on learn.netdata.cloud

```
Alerts & Notifications
├── Understanding Alerts
│   ├── What is a Netdata Alert
│   ├── Alert Types: alarm vs template
│   └── Where Alerts Live (Files, Agent, Cloud)
│
├── Creating and Managing Alerts
│   ├── Quick Start: Create Your First Alert
│   ├── Creating and Editing Alerts via Configuration Files
│   ├── Creating and Editing Alerts via Netdata Cloud
│   ├── Managing Stock versus Custom Alerts
│   └── Reloading and Validating Alert Configuration
│
├── Alert Configuration Syntax
│   ├── Alert Definition Lines (Minimal and Full Forms)
│   ├── Lookup and Time Windows
│   ├── Calculations and Transformations
│   ├── Expressions, Operators, and Functions
│   ├── Variables and Special Symbols
│   └── Optional Metadata: class, type, component, and tags
│
├── Controlling Alerts and Noise
│   ├── Disabling Alerts (Locally or Per-Host)
│   ├── Silencing versus Disabling Alerts
│   ├── Silencing in Netdata Cloud
│   └── Reducing Flapping and Noise
│
├── Receiving Notifications
│   ├── Notification Concepts: Agent, Parent, and Cloud
│   ├── Configuring Agent and Parent Notifications (Local Methods)
│   ├── Configuring Cloud Notifications
│   ├── Controlling Who Gets Notified
│   ├── Testing and Troubleshooting Notification Delivery
│   │
│   ├── Agent Dispatched Notifications          ← NESTED HERE
│   │   ├── Agent Dispatched Notifications     (main doc)
│   │   ├── Agent Notifications Reference
│   │   ├── Alerta
│   │   ├── AWS SNS
│   │   ├── Custom
│   │   ├── Discord
│   │   ├── Dynatrace
│   │   ├── Email
│   │   ├── Flock
│   │   ├── Gotify
│   │   ├── iLert
│   │   ├── IRC
│   │   ├── Kavenegar
│   │   ├── Matrix
│   │   ├── Microsoft Teams
│   │   ├── MessageBird
│   │   ├── NTFY
│   │   ├── OpsGenie
│   │   ├── PagerDuty
│   │   ├── Prowl
│   │   ├── Pushbullet
│   │   ├── Pushover
│   │   ├── Rocket.Chat
│   │   ├── Signal4
│   │   ├── Slack
│   │   ├── SMSEagle
│   │   ├── SMS
│   │   ├── Syslog
│   │   ├── Telegram
│   │   └── Twilio
│   │
│   └── Centralized Cloud Notifications        ← NESTED HERE
│       ├── Centralized Cloud Notifications   (main doc)
│       ├── Cloud Notifications Reference
│       ├── Amazon SNS
│       ├── Discord
│       ├── iLert
│       ├── Mattermost
│       ├── Microsoft Teams
│       ├── Netdata Mobile App
│       ├── OpsGenie
│       ├── PagerDuty
│       ├── Rocket.Chat
│       ├── ServiceNow
│       ├── Slack
│       ├── Splunk
│       ├── Splunk VictorOps
│       ├── Telegram
│       └── Webhook
│
├── Alert Examples and Common Patterns
├── Troubleshooting Alert Behaviour
├── Essential Alert Patterns
├── APIs for Alerts and Events
├── Netdata Cloud Alert and Events Features
├── Best Practices for Alerting
│   ├── Designing Useful Alerts
│   ├── Notification Strategy and On-Call Hygiene
│   ├── Maintaining Alert Configurations Over Time
│   ├── Patterns for Large and Distributed Environments
│   └── SLIs, SLOs, and How They Relate to Alerts
│
└── Alerts and Notifications Architecture
    ├── Alert Evaluation Architecture
    ├── Alert State Machine and Lifecycle
    ├── Notification Dispatch Flow
    ├── Configuration Layers and Precedence
    └── Scaling Alerting in Complex Topologies
```

---

## Lines Edited in map.csv

| Area | Lines | Notes |
|------|-------|-------|
| Agent Dispatched | ~190-220 | 28 entries added |
| Cloud Dispatched | ~221-239 | 15 entries added |

**Structure:** Nested under `Alerts & Notifications/Receiving Notifications/Agent Dispatched Notifications` and `.../Centralized Cloud Notifications`

---

## That's It

map.csv now reflects the correct structure with integrations nested under Receiving Notifications.