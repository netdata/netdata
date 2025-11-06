# Collector configuration

Find available collectors in the [Collecting Metrics](/src/collectors/README.md) guide and on our [Integrations page](https://www.netdata.cloud/integrations). Each collector's documentation includes detailed setup instructions and configuration options.

:::tip

Enable and configure Go collectors directly through the UI using the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md).

:::

## Configuration file basics

Collectors use YAML configuration files stored in `/etc/netdata/`. The configuration structure varies by collector type:

| Collector Type | Plugin Config | Collector Configs |
|---------------|---------------|-------------------|
| Go collectors | `/etc/netdata/go.d.conf` | `/etc/netdata/go.d/*.conf` |
| Python collectors | `/etc/netdata/python.d.conf` | `/etc/netdata/python.d/*.conf` |
| Bash collectors | `/etc/netdata/charts.d.conf` | `/etc/netdata/charts.d/*.conf` |

### Single-job configuration

Use this format when monitoring one instance of a service:

```yaml
update_every: 5
timeout: 10
url: http://localhost:8080
username: monitor
password: secret
```

### Multi-job configuration

Monitor multiple instances by defining separate jobs with unique names:

```yaml
# Module defaults (applied to all jobs)
update_every: 5
timeout: 10

# Job 1: Production database
production:
  url: http://prod.example.com:8080
  username: monitor
  password: prod_secret

# Job 2: Staging database  
staging:
  url: http://staging.example.com:8080
  username: monitor
  password: staging_secret
  update_every: 10  # Override module default
```

### Common configuration options

| Option | Description | Default |
|--------|-------------|---------|
| `update_every` | Collection frequency in seconds | 1 |
| `priority` | Dashboard chart position | 70000 |
| `autodetection_retry` | Retry interval for auto-discovery (0 disables) | 0 |
| `timeout` | Connection timeout in seconds | 1 |

## Configure authentication

:::note

Many collectors require credentials to access services. Configure authentication based on your service type.

:::

### Username and password

```yaml
mysql_local:
  username: netdata
  password: monitor_password
  host: localhost
  port: 3306
```

### API keys and tokens

```yaml
prometheus_remote:
  url: http://prometheus.example.com:9090
  headers:
    Authorization: Bearer YOUR_API_KEY
```

### Certificate-based authentication

```yaml
elasticsearch_secure:
  url: https://elasticsearch.example.com:9200
  tls_cert: /path/to/client.crt
  tls_key: /path/to/client.key
  tls_ca: /path/to/ca.crt
  tls_skip_verify: false
```

:::warning 

- Set restrictive file permissions on configuration files containing credentials:
  ```bash
  sudo chmod 640 /etc/netdata/go.d/mysql.conf
  sudo chown root:netdata /etc/netdata/go.d/mysql.conf
  ```
- Create dedicated monitoring accounts with read-only privileges
- Avoid storing passwords in plaintext when possible
  
:::

## Auto-discovery

Many collectors automatically discover services on your system without manual configuration. The collector scans for running services (by port, process, or file) and generates configuration automatically.

### View auto-discovered services

Run the collector in debug mode to see what it discovered:

```bash
sudo su -s /bin/bash netdata
/usr/libexec/netdata/plugins.d/go.d.plugin -d -m mysql
```

Look for discovery messages in the output:

```
[mysql] auto-discovery: found MySQL on localhost:3306
[mysql] using auto-generated configuration
```

### Override auto-discovered settings

Create a configuration file to customize auto-discovered services:

```bash
sudo ./edit-config go.d/mysql.conf
```

Add your custom settings:

```yaml
mysql_local:
  host: localhost
  port: 3306
  username: custom_user
  password: custom_password
```
:::note

Your manual configuration takes precedence over auto-discovered settings.

:::

### Disable auto-discovery

Disable auto-discovery for a specific collector by setting `autodetection_retry` to `0`:

```yaml
autodetection_retry: 0
```

## Enable or disable collectors

Most collectors and plugins run by default. Disable them selectively to optimize performance.

### Disable plugins

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config)
2. Navigate to the `[plugins]` section
3. Uncomment the plugin line and set it to `no`:

```text
[plugins]
    proc = yes
    python.d = no
```

### Disable specific collectors

1. Open the plugin configuration file:

```bash
sudo ./edit-config go.d.conf
```

2. Set the collector to `no`:

```yaml
modules:
    mysql: no
```

3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent

## Adjust data collection frequency

Modify collection intervals to balance metric granularity with resource usage.

### Global frequency

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config)
2. Set the `update every` value in seconds (default is `1`):

```text
[global]
    update every = 2
```

3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent

### Plugin-specific frequency

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config)
2. Set the frequency for the plugin:

```text
[plugin:apps]
    update every = 5
```

3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent

### Collector-specific frequency

Set `update_every` in the collector's configuration file:

```yaml
mysql_local:
  update_every: 10
  host: localhost
```

## Configuration examples

### Monitor a remote MySQL database

```yaml
# /etc/netdata/go.d/mysql.conf
mysql_remote:
  host: mysql.example.com
  port: 3306
  username: netdata
  password: monitor_pass
  timeout: 5
  update_every: 10
```

### Monitor multiple web servers

```yaml
# /etc/netdata/go.d/web_log.conf
nginx_prod:
  path: /var/log/nginx/prod-access.log

nginx_staging:
  path: /var/log/nginx/staging-access.log
  update_every: 5

apache_main:
  path: /var/log/apache2/access.log
```

### Configure custom ports and timeouts

```yaml
# /etc/netdata/go.d/prometheus.conf
prometheus_custom:
  url: http://localhost:9091/metrics
  timeout: 10
  update_every: 5
  not_follow_redirects: true
```

## Validate configuration

Test configuration changes before applying them to production.

### Check YAML syntax

Verify your configuration file has valid YAML syntax:

```bash
# Install yamllint if needed
sudo apt-get install yamllint

# Validate syntax
yamllint /etc/netdata/go.d/mysql.conf
```

### Test collector configuration

Run the collector in debug mode to verify configuration:

```bash
sudo su -s /bin/bash netdata
/usr/libexec/netdata/plugins.d/go.d.plugin -d -m mysql
```

Look for configuration errors:
- `failed to load config`: Configuration file syntax error
- `failed to create job`: Invalid job configuration
- `connection refused`: Network or authentication issue

### Verify active configuration

Check which configuration file the collector is using:

```bash
# Run in debug mode and look for "loaded config from" messages
/usr/libexec/netdata/plugins.d/go.d.plugin -d -m mysql | grep "config"
```

## Troubleshoot collectors

Run collectors in debug mode to identify configuration and connection issues:

1. Navigate to the plugins directory (check `plugins directory` in `netdata.conf` if not found):

```bash
cd /usr/libexec/netdata/plugins.d/
```

2. Switch to the netdata user:

```bash
sudo su -s /bin/bash netdata
```

3. Run the collector in debug mode:

```bash
# Go collectors
./go.d.plugin -d -m <MODULE_NAME>

# Python collectors
./python.d.plugin <MODULE_NAME> debug trace

# Bash collectors
./charts.d.plugin debug 1 <MODULE_NAME>
```

<details>
<summary><strong>Collector shows "failed to load config"</strong></summary>

Your configuration file has invalid YAML syntax. Validate it with yamllint:

```bash
yamllint /etc/netdata/go.d/mysql.conf
```

Common YAML errors:
- Inconsistent indentation (use spaces, not tabs)
- Missing colons after keys
- Unquoted special characters in values
</details>

<details>
<summary><strong>Debug output shows "connection refused"</strong></summary>

The collector cannot reach the service. Verify:

1. The service is running:
   ```bash
   sudo systemctl status mysql
   ```

2. The host and port are correct in your configuration

3. Network connectivity from the Netdata host:
   ```bash
   telnet localhost 3306
   ```

4. Firewall rules allow the connection
</details>

<details>
<summary><strong>Collector shows "authentication failed"</strong></summary>

Check your credentials:

1. Verify username and password are correct
2. Test authentication manually:
   ```bash
   mysql -u netdata -p -h localhost
   ```

3. Ensure the monitoring user has required privileges
4. Check for special characters that need quoting in YAML
</details>

<details>
<summary><strong>Debug output shows "permission denied"</strong></summary>

The netdata user cannot read the configuration file. Fix permissions:

```bash
sudo chmod 640 /etc/netdata/go.d/mysql.conf
sudo chown root:netdata /etc/netdata/go.d/mysql.conf
```

For log file collectors, ensure netdata can read the log files:

```bash
sudo usermod -a -G adm netdata
```
</details>

<details>
<summary><strong>Collector shows timeout errors</strong></summary>

The service is responding too slowly. Increase the timeout in your configuration:

```yaml
mysql_local:
  timeout: 10
  host: localhost
```

If timeouts persist:
- Check service performance and load
- Verify network latency
- Ensure the service isn't overloaded
</details>

<details>
<summary><strong>Configuration changes don't take effect</strong></summary>

Verify the configuration is being applied:

1. Check you edited the correct file location
2. Verify file permissions allow netdata to read it
3. Confirm you restarted the Agent:
   ```bash
   sudo systemctl restart netdata
   ```

4. Run debug mode to see which configuration loaded:
   ```bash
   ./go.d.plugin -d -m mysql | grep "config"
   ```
</details>

<details>
<summary><strong>Collector not appearing in debug output</strong></summary>

The collector might be disabled. Check the plugin configuration:

```bash
sudo ./edit-config go.d.conf
```

Ensure the collector is enabled:

```yaml
modules:
  mysql: yes
```
</details>

<details>
<summary><strong>Need to reset to default configuration</strong></summary>

Remove your custom configuration to restore defaults:

```bash
sudo rm /etc/netdata/go.d/mysql.conf
sudo systemctl restart netdata
```

The collector will use built-in defaults or attempt auto-discovery.
</details>

## Configuration precedence

When multiple configuration sources exist, Netdata applies them in this order (highest to lowest priority):

1. Dynamic Configuration (UI-based, go.d collectors only)
2. User configuration files (`/etc/netdata/`)
3. Stock configuration (shipped with Netdata)
4. Auto-discovery (if no manual configuration exists)

Manual configuration in `/etc/netdata/` always overrides auto-discovery and stock configuration.

## Advanced configuration

### Required Linux capabilities

Some collectors require specific Linux capabilities:

| Collector | Capability | Purpose |
|-----------|-----------|---------|
| Ping | `CAP_NET_RAW` | Send ICMP packets |
| Wireguard | `CAP_NET_ADMIN` | Read network configuration |
| Filecheck | `CAP_DAC_READ_SEARCH` | Read protected files |

Grant capabilities if needed:

```bash
sudo setcap cap_net_raw+ep /usr/libexec/netdata/plugins.d/go.d.plugin
```

### Reload vs restart

Most configuration changes require restarting the Agent:

```bash
sudo systemctl restart netdata
```

Go collectors using Dynamic Configuration apply changes immediately without restart.

## Get help

Need assistance with collector configuration? Join our [Discord community](https://discord.com/invite/2mEmfW735j) for expert help.
