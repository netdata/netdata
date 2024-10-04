# Asterisk collector

Monitors [Asterisk](https://www.asterisk.org/) dialplan application's statistics.

## Requirements

- Asterisk [integrated with StatsD](https://www.asterisk.org/integrating-asterisk-with-statsd/).

## Configuration

Netdata ships
with [asterisk.conf](https://github.com/netdata/netdata/blob/master/src/collectors/statsd.plugin/asterisk.conf) with
preconfigured charts.

To receive Asterisk metrics in Netdata, uncomment the following lines in the `/etc/asterisk/statsd.conf` file:

```ini
[general]
enabled = yes                   ; When set to yes, statsd support is enabled
server = 127.0.0.1              ; server[:port] of statsd server to use.
                                ; If not specified, the port is 8125
prefix = asterisk               ; Prefix to prepend to all metrics
```

> See [statsd.conf.sample](https://github.com/asterisk/asterisk/blob/master/configs/samples/statsd.conf.sample) for all available options.

## Charts and metrics

<details><summary>Click to see screenshots of the charts.</summary>

![image](https://user-images.githubusercontent.com/2662304/158055351-fcc7a7fb-9b95-4656-bdc6-2e5f5a909215.png)
![image](https://user-images.githubusercontent.com/2662304/158055367-cfd25cd5-d71a-4bab-8cd1-bfcc47bc7312.png)

</details>

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
