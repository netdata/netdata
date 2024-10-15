[% if entry.integration_type == 'collector' %]
[% if entry.meta.plugin_name is in(['go.d.plugin', 'python.d.plugin', 'charts.d.plugin']) %]
## Troubleshooting

### Debug Mode

[% if entry.meta.plugin_name == 'go.d.plugin' %]
**Important**: Debug mode is not supported for data collection jobs created via the UI using the Dyncfg feature.
[% endif %]

To troubleshoot issues with the `[[ entry.meta.module_name ]]` collector, run the `[[ entry.meta.plugin_name ]]` with the debug option enabled. The output
should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on
  your system, open `netdata.conf` and look for the `plugins` setting under `[directories]`.

  ```bash
  cd /usr/libexec/netdata/plugins.d/
  ```

- Switch to the `netdata` user.

  ```bash
  sudo -u netdata -s
  ```

[% if entry.meta.plugin_name == 'go.d.plugin' %]
- Run the `go.d.plugin` to debug the collector:

  ```bash
  ./go.d.plugin -d -m [[ entry.meta.module_name ]]
  ```

[% elif entry.meta.plugin_name == 'python.d.plugin' %]
- Run the `python.d.plugin` to debug the collector:

  ```bash
  ./python.d.plugin [[ entry.meta.module_name ]] debug trace
  ```

[% elif entry.meta.plugin_name == 'charts.d.plugin' %]
- Run the `charts.d.plugin` to debug the collector:

  ```bash
  ./charts.d.plugin debug 1 [[ entry.meta.module_name ]]
  ```

[% endif %]
### Getting Logs

If you're encountering problems with the `[[ entry.meta.module_name ]]` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues.  These messages should provide clues about the root cause of the problem.

#### System with systemd

Use the following command to view logs generated since the last Netdata service restart:

```bash
journalctl _SYSTEMD_INVOCATION_ID="$(systemctl show --value --property=InvocationID netdata)" --namespace=netdata --grep [[ entry.meta.module_name ]]
```

#### System without systemd

Locate the collector log file, typically at `/var/log/netdata/collector.log`, and use `grep` to filter for collector's name:

```bash
grep [[ entry.meta.module_name ]] /var/log/netdata/collector.log
```

**Note**: This method shows logs from all restarts. Focus on the **latest entries** for troubleshooting current issues.

#### Docker Container

If your Netdata runs in a Docker container named "netdata" (replace if different), use this command:

```bash
docker logs netdata 2>&1 | grep [[ entry.meta.module_name ]]
```

[% else %]
[% if entry.troubleshooting.problems.list %]
## Troubleshooting

[% endif %]
[% endif %]
[% elif entry.integration_type == 'cloud_notification' %]
[% if entry.troubleshooting.problems.list %]
## Troubleshooting

[% endif %]
[% elif entry.integration_type == 'agent_notification' %]
## Troubleshooting

### Test Notification

You can run the following command by hand, to test alerts configuration:

```bash
# become user netdata
sudo su -s /bin/bash netdata

# enable debugging info on the console
export NETDATA_ALARM_NOTIFY_DEBUG=1

# send test alarms to sysadmin
/usr/libexec/netdata/plugins.d/alarm-notify.sh test

# send test alarms to any role
/usr/libexec/netdata/plugins.d/alarm-notify.sh test "ROLE"
```

Note that this will test _all_ alert mechanisms for the selected role.

[% elif entry.integration_type == 'exporter' %]
[% if entry.troubleshooting.problems.list %]
## Troubleshooting

[% endif %]
[% endif %]
[% for item in entry.troubleshooting.problems.list %]
### [[ item.name ]]

[[ description ]]

[% endfor %]
