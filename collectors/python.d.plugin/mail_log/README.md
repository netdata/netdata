# mail_log

Message transfer agent (MTA) log monitoring module.

Charts:

1.  **Incoming connections** - incoming SMTP connections per second,
2.  **Incoming email statuses** - accepted, greylisted, temporarily rejected and permanently rejected emails per second,
3.  **Incoming email status codes** - detailed status codes of incoming emails,
4.  **Outgoing email statuses** - sent, deferred and bounced outgoing emails per second,
5.  **Outgoing email status codes** - detailed status codes of outgoing emails.

Currently supported mail transfer agents:

*   Postfix

## Configuration

Sample/default config:
```yaml
local:
  name: 'local'
  path: '/var/log/mail.log'
  type: 'postfix'
  greylist_status: 'Try again later'
```

Depending on your linux distribution you might need to install and configure a syslog daemon and log rotation to allow
Netdata read MTA logs.
