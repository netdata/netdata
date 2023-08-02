[% if entry.integration_type == 'collector' %]
[% if entry.meta.plugin_name == 'go.d.plugin' %]
## Troubleshooting

### Debug Mode

To troubleshoot issues with the `[[ entry.module_name ]]` collector, run the `go.d.plugin` with the debug option enabled. The output
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

- Run the `go.d.plugin` to debug the collector:

  ```bash
  ./go.d.plugin -d -m [[ entry.module_name ]]
  ```

[% elif entry.meta.plugin_name == 'python.d.plugin' %]
## Troubleshooting

### Debug Mode

To troubleshoot issues with the `[[ entry.module_name ]]` collector, run the `python.d.plugin` with the debug option enabled. The output
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

- Run the `python.d.plugin` to debug the collector:

  ```bash
  ./python.d.plugin [[ entry.module_name ]] debug trace
  ```

[% elif entry.meta.plugin_name == 'charts.d.plugin' %]
## Troubleshooting

### Debug Mode

To troubleshoot issues with the `[[ entry.module_name ]]` collector, run the `charts.d.plugin` with the debug option enabled. The output
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

- Run the `charts.d.plugin` to debug the collector:

  ```bash
  ./charts.d.plugin debug 1 [[ entry.module_name ]]
  ```

[% else %]
[% if entry.troubleshooting.problems.list %]
## Troubleshooting

[% endif %]
[% endif %]
[% elif entry.integration_type == 'notification' %]
[% if 'cloud-notifications' in entry._src_path %]
[% if entry.troubleshooting.problems.list %]
## Troubleshooting

[% endif %]
[% else %]
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
[% endif %]
