# Registry Configuration Reference

You can configure Netdata's **central Registry** to provide unified cross-server dashboards. Together with certain browser features, it allows Netdata to provide these dashboards. The Registry operates with [minimal data transfer](/src/registry/README.md#communication-with-the-registry), with all communication occurring directly between your web browser and the Registry.

:::info

Read more about it in the [Registry overview](/src/registry/README.md).

:::

## Configure a Custom Registry

Any Netdata Agent can function as a Registry.

**Set up your Registry node:**

1. Modify `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config):

   ```text
   [registry]
       enabled = yes
       registry to announce = http://your.registry:19999
   ```

2. [Restart the Agent](/docs/netdata-agent/start-stop-restart.md) for the changes to take effect.

3. Configure all other Agents to use your custom Registry instead of the default one. For each Agent, modify `netdata.conf`:

   ```text
   [registry]
      enabled = no
      registry to announce = http://your.registry:19999
   ```

4. To improve node identification in your dashboard, you can assign custom names to each Agent (optional):

   ```text
   [registry]
      registry hostname = Group1 - Master DB
   ```

## Configure Registry Access Control

You can restrict Registry access to specific IP addresses or hostnames using [simple pattern](/src/libnetdata/simple_pattern/README.md) matching:

```text
[registry]
    allow from = *
```

:::tip

For example, `allow from = !10.1.2.3 10.*` allows all IPs in the `10.*` range except `10.1.2.3`.

:::

### Access Control Considerations

- Registry access rules work in conjunction with the main API access control (`[web].allow connections from`). IPs must be allowed by both settings to access the Registry.
- Patterns can match against IP addresses or host FQDNs. For hostname matching, the system performs both reverse and forward DNS lookups to prevent DNS spoofing.

### DNS Resolution Settings

DNS resolution for pattern matching can impact performance on systems handling many connections. You can control this behavior using:

```text
[registry]
    allow by dns = heuristic
```

| Option | Description |
|--------|-------------|
| `yes` | Enables hostname pattern matching using DNS |
| `no` | Restricts patterns to match IP addresses only |
| `heuristic` | Automatically determines whether to use DNS based on pattern syntax (presence of `:` or letters) |

## Registry Database Location

The Registry maintains its data in two text-based database files located at `/var/lib/netdata/registry/`.

| File | Purpose | Behavior |
|------|---------|----------|
| `registry-log.db` | Records all real-time Registry operations | Captures every modification to the Registry as it occurs |
| `registry.db` | Stores the consolidated Registry data | Updates after every `[registry].registry save db every new entries` entries in the transaction log, at which point the main database is refreshed and the transaction log is cleared |

## Configure Cookie Security Settings

By default, the Netdata Agent's web server sets `SameSite=none` and `Secure` attributes for its cookies. If these security settings interfere with accessing your Agent dashboard or Netdata Cloud, you can disable them.

To modify cookie settings, edit `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config):

```text
[registry]
    enable cookies SameSite and Secure = no
```

:::warning

Disabling these security attributes may affect browser compatibility and security. Only disable them if you're experiencing specific access issues.

:::

## Troubleshoot Registry Issues

### Verify Registry Configuration

The Registry URL must point to a valid Netdata dashboard where the Registry is enabled (`[registry].enabled = yes`). You can verify your Registry configuration by accessing its URL directly in your web browser—it should display the dashboard of the Netdata Agent running the Registry.

### Cookie Requirements

The Registry relies on third-party cookies to function properly. The Registry sets these cookies while you're viewing dashboards from other Netdata Agents.

When a new browser first connects, the Registry performs a cookie compatibility check through the following process:

- Set a test cookie
- Redirect the browser back to verify the cookie

### Debug Connection Problems

If cookies are disabled or blocked, this process fails after several redirects with an error similar to:

```text
ERROR 409: Cannot ACCESS netdata registry: https://registry.my-netdata.io responded with: {"status":"redirect","registry":"https://registry.my-netdata.io"}
```

To view these error messages, open your browser's developer console (typically F12).