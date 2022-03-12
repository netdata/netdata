<!--
title: "Asterisk monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/statsd.plugin/asterisk.md

sidebar_label: "Asterisk"
-->

# Asterisk monitoring with Netdata

Monitors [Asterisk](https://www.asterisk.org/) dialplan application's statistics.

## Requirements

- Asterisk [integrated with StatsD](https://www.asterisk.org/integrating-asterisk-with-statsd/).

## Configuration

Netdata ships
with [asterisk.conf](https://github.com/netdata/netdata/blob/master/collectors/statsd.plugin/asterisk.conf) with
preconfigured charts.

You only need to configure Asterisk to send statistics to Netdata. The following suffices for `statsd.conf`:

```ini
[general]
enabled = true
server = 127.0.0.1 ; adjust the 'server' value if Netdata is running on a remote machine.
```

> See [statsd.conf.sample](https://github.com/asterisk/asterisk/blob/master/configs/samples/statsd.conf.sample) for all available options.

## Metrics

Mapping Asterisk StatsD metrics and Netdata charts.

| Chart                                                | Metrics                                    |
|------------------------------------------------------|--------------------------------------------|
| Active Channels                                      | asterisk.channels.count                    |
| Active Endpoints                                     | asterisk.endpoints.count                   |
| Active Endpoints by Status                           | asterisk.endpoints.state.*                 |
| Active SIP channels by endpoint                      | asterisk.endpoints.SIP.*.channels          |
| Active PJSIP channels by endpoint                    | asterisk.endpoints.PJSIP.*.channels        |
| Distribution of Dial Statuses                        | asterisk.dialstatus.*                      |
| Asterisk Channels Call Duration                      | asterisk.channels.calltime                 |
| Distribution of Hangup Causes                        | asterisk.hangupcause.*                     |
| Distribution of Hangup Causes for ANSWERed calls     | asterisk.dialhangupcause.ANSWER.*          |
| Distribution of Hangup Causes for BUSY calls         | asterisk.dialhangupcause.BUSY.*            |
| Distribution of Hangup Causes for CANCELled calls    | asterisk.dialhangupcause.CANCEL.*          |
| Distribution of Hangup Causes for CHANUNVAILed calls | asterisk.dialhangupcause.CHANUNAVAIL.*     |
| Distribution of Hangup Causes for CONGESTIONed calls | asterisk.dialhangupcause.CONGESTION.*      |
| Asterisk Dialplan Events                             | asterisk.stasis.message.ast_channel_*_type |
| Asterisk PJSIP Peers Qualify                         | asterisk.PJSIP.contacts.*.rtt              |
