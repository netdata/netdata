[# Jinja template: integrations/templates/troubleshooting.md
   Renders the h2 'Troubleshooting' section as up to four h3 groups, in this order:
   - 'Diagnostics' (collectors of go.d/python.d/charts.d only): the plugin debug command and the log commands.
   - 'Test Notification' (agent notifications only).
   - 'Known Errors': one h4 per `troubleshooting.errors.list[]` entry; heading = the error text, body = When / Cause / Fix.
   - 'Other Problems': one h4 per legacy `troubleshooting.problems.list[]` entry.
   Content rules for the entries: .agents/skills/project-collector-metadata/troubleshooting.md #]
[% set has_diagnostics = entry.integration_type == 'collector' and entry.meta.plugin_name is in(['go.d.plugin', 'python.d.plugin', 'charts.d.plugin']) %]
[% set has_test_notification = entry.integration_type == 'agent_notification' %]
[% set has_errors = entry.troubleshooting is defined and entry.troubleshooting.errors is defined and entry.troubleshooting.errors.list %]
[% set has_problems = entry.troubleshooting is defined and entry.troubleshooting.problems is defined and entry.troubleshooting.problems.list %]
[% if has_diagnostics or has_test_notification or has_errors or has_problems %]
## Troubleshooting

[% endif %]
[% if has_diagnostics %]
### Diagnostics

#### Debug Mode

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

  To debug a specific job:

  ```bash
  ./go.d.plugin -d -m [[ entry.meta.module_name ]] -j jobName
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
#### Getting Logs

If you're encountering problems with the `[[ entry.meta.module_name ]]` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues.  These messages should provide clues about the root cause of the problem.

##### System with systemd

Use the following command to view logs generated since the last Netdata service restart:

```bash
journalctl _SYSTEMD_INVOCATION_ID="$(systemctl show --value --property=InvocationID netdata)" --namespace=netdata --grep [[ entry.meta.module_name ]]
```

##### System without systemd

Locate the collector log file, typically at `/var/log/netdata/collector.log`, and use `grep` to filter for collector's name:

```bash
grep [[ entry.meta.module_name ]] /var/log/netdata/collector.log
```

**Note**: This method shows logs from all restarts. Focus on the **latest entries** for troubleshooting current issues.

##### Docker Container

If your Netdata runs in a Docker container named "netdata" (replace if different), use this command:

```bash
docker logs netdata 2>&1 | grep [[ entry.meta.module_name ]]
```

[% endif %]
[% if has_test_notification %]
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

[% endif %]
[% if has_errors %]
### Known Errors

[% for item in entry.troubleshooting.errors.list %]
#### [[ item.error ]]

[% if item.when %]
**When**

[[ item.when ]]

[% endif %]
**Cause**

[[ item.cause ]]

**Fix**

[[ item.fix ]]

[% if item.source %]
Reported in [[ item.source ]].

[% endif %]
[% endfor %]
[% endif %]
[% if has_problems %]
### Other Problems

[% for item in entry.troubleshooting.problems.list %]
#### [[ item.name ]]

[[ item.description ]]

[% endfor %]
[% endif %]
