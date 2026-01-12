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

```conf
# In local health configuration
template: critical_service
    on: systemd.service_unit_state
    lookup: average -1m of status
    every: 1m
    crit: $this == 0
    to: ops-pager@company.com
    from: ops-team@company.com
```

## 5.4.4 Related Sections

- **5.2 Agent and Parent Notifications** - Local routing
- **5.5 Testing and Troubleshooting** - Debugging delivery issues
- **12.2 Notification Strategy** - On-call best practices