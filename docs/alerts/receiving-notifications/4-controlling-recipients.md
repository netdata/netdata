# 5.4 Controlling Who Gets Notified

Recipient routing determines which people or teams receive alerts based on severity, role, or alert-specific rules.

### 5.4.1 Severity-to-Recipient Mapping

In Cloud integrations, you can configure which severities trigger notifications:

```yaml
integration: Slack #alerts
severity:
  critical:
    - "#urgent"
    - on-call-pager
  warning:
    - "#alerts"
  clear:
    - "#alerts"
    critical:
      - "#urgent"
      - on-call-pager
    warning:
      - "#alerts"
    clear:
      - "#alerts"
```

### 5.4.2 Using Cloud Roles

Assign roles to users in Netdata Cloud:

1. Navigate to **Settings** → **Users**
2. Select a user
3. Assign roles: `Admin`, `Member`, `Viewer`, or custom roles

Then configure notification routing by role:

```yaml
integration: Email ops-team@company.com
  role:
    - name: sre-on-call
      severity: [critical, warning]
    - name: manager
      severity: [critical]
```

### 5.4.3 Alert-Specific Routing

Override the default recipient for specific alerts by setting `to:` in your local health configuration:

```conf
# In local health configuration (override stock alert recipient)
template: systemd_service_unit_failed_state
      on: systemd.service_unit_state
   class: Errors
    type: Linux
component: Systemd units
chart labels: unit_name=!*
     calc: $failed
    units: state
    every: 10s
     warn: $this != nan AND $this == 1
    delay: down 5m multiplier 1.5 max 1h
  summary: systemd unit ${label:unit_name} state
     info: systemd service unit in the failed state
       to: ops-team@company.com
```

## 5.4.4 Related Sections

- **[5.2 Agent and Parent Notifications](2-agent-parent-notifications.md)** - Local routing
- **[5.5 Testing and Troubleshooting](5-testing-troubleshooting.md)** - Debugging delivery issues
- **[12.2 Notification Strategy and On-Call Hygiene](../best-practices/2-notification-strategy.md)** - On-call best practices