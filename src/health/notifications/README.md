# Agent Alert Notifications

Netdata's Agent can send alert notifications directly from each node. It supports a wide range of services, multiple recipients, and role-based routing.

---

## How It Works

The Agent uses a notification script defined in `netdata.conf` under the `[health]` section:

```ini
script to execute on alarm = /usr/libexec/netdata/plugins.d/alarm-notify.sh
```

The default script is `alarm-notify.sh`.

This script handles:

- Multiple recipients
- Multiple notification methods
- Role-based routing (e.g., `sysadmin`, `webmaster`, `dba`)

---

## Quick Setup

:::tip Recommended
Use the `edit-config` script to safely edit configuration files. It automatically creates the necessary files in the right place and opens them in your editor.
→ [Learn how to use `edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config)
:::

1. Open the Agent’s health notification config:
   ```bash
   sudo ./edit-config health_alarm_notify.conf
   ```

2. Set up the required API keys or credentials for the service you want to use.

3. Define recipients per **role** (see below).

4. Restart the Agent for changes to take effect:
   ```bash
   sudo systemctl restart netdata
   ```

---

## Example: Alert with Role-Based Routing

Here’s an example alert assigned to the `sysadmin` role from the `ram.conf` file:

```ini
alarm: ram_in_use
   on: system.ram
class: Utilization
 type: System
component: Memory
     os: linux
  hosts: *
   calc: $used * 100 / ($used + $cached + $free + $buffers)
  units: %
  every: 10s
   warn: $this > (($status >= $WARNING)  ? (80) : (90))
   crit: $this > (($status == $CRITICAL) ? (90) : (98))
  delay: down 15m multiplier 1.5 max 1h
   info: system memory utilization
     to: sysadmin
```

Then, in `health_alarm_notify.conf`, you assign recipients per notification method:

```ini
role_recipients_email[sysadmin]="admin1@example.com admin2@example.com"
role_recipients_slack[sysadmin]="#alerts #infra"
```

---

## Configuration Options

### Recipients Per Role

Define who receives alerts and how:

```ini
role_recipients_email[sysadmin]="team@example.com"
role_recipients_telegram[webmaster]="123456789"
role_recipients_slack[dba]="#database-alerts"
```

Use spaces to separate multiple recipients.

To disable a notification method for a role, use:

```ini
role_recipients_email[sysadmin]="disabled"
```

If left empty, the default recipient for that method is used.

---

### Alert Severity Filtering

You can limit certain recipients to only receive **critical** alerts:

```ini
role_recipients_email[sysadmin]="user1@example.com user2@example.com|critical"
```

This setup:

- Sends all alerts to `user1@example.com`
- Sends only critical-related alerts to `user2@example.com`

Works for all supported methods: email, Slack, Telegram, Twilio, Discord, etc.

---

### Proxy Settings

To send notifications via a proxy, set these environment variables:

```bash
export http_proxy="http://10.0.0.1:3128/"
export https_proxy="http://10.0.0.1:3128/"
```

---

### Notification Images

By default, Netdata includes public image URLs in notifications (hosted by the global Registry).

To use custom image paths:

```ini
images_base_url="http://my.public.netdata.server:19999"
```

---

### Custom Date Format

Change the timestamp format in notifications:

```ini
date_format="+%F %T%:z"   # Example: RFC 3339
```

Common formats:

| Format             | String                      |
|--------------------|-----------------------------|
| ISO 8601           | `+%FT%T%z`                  |
| RFC 5322           | `+%a, %d %b %Y %H:%M:%S %z` |
| RFC 3339           | `+%F %T%:z`                 |
| Local time         | `+%x %X`                    |
| ANSI C / asctime() | *(leave empty)*             |

→ See `man date` for more formatting options.

---

### Hostname Format

By default, Netdata uses the short hostname in notifications.

To use the fully qualified domain name (FQDN), set:

```ini
use_fqdn=YES
```

If you’ve set a custom hostname in `netdata.conf`, that value takes priority.

---

## Testing Your Notification Setup

You can test alert notifications manually.

```bash
# Switch to the Netdata user
sudo su -s /bin/bash netdata

# Enable debugging
export NETDATA_ALARM_NOTIFY_DEBUG=1

# Test default role (sysadmin)
./plugins.d/alarm-notify.sh test

# Test specific role
./plugins.d/alarm-notify.sh test "webmaster"
```

:::info Using a custom Registry?
If you’re running your own Netdata Registry, set:

```bash
export NETDATA_REGISTRY_URL="https://your.registry.url"
```

before testing.
:::

### Debugging with Trace

To see full execution output:

```bash
bash -x ./plugins.d/alarm-notify.sh test
```

Then look for the internal calls and re-run the one you want to trace in more detail.

---

## Related Docs

- [How to configure alerts](/src/health/REFERENCE.md)
- [Notification methods list](/docs/alerts-and-notifications/notifications/README.md#notification-methods)
- [Netdata configuration basics](/docs/netdata-agent/configuration/README.md)