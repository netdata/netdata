<!--
title: "Fail2ban monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/fail2ban/README.md"
sidebar_label: "Fail2ban"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Apps"
-->

# Fail2ban collector

Monitors the fail2ban log file to show all bans for all active jails.

## Requirements

The `fail2ban.log` file must be readable by the user `netdata`:

- change the file ownership and access permissions.
- update `/etc/logrotate.d/fail2ban` to persists the changes after rotating the log file.

<details>
  <summary>Click to expand the instruction.</summary>

To change the file ownership and access permissions, execute the following:

```shell
sudo chown root:netdata /var/log/fail2ban.log
sudo chmod 640 /var/log/fail2ban.log
```

To persist the changes after rotating the log file, add `create 640 root netdata` to the `/etc/logrotate.d/fail2ban`:

```shell
/var/log/fail2ban.log {

    weekly
    rotate 4
    compress

    delaycompress
    missingok
    postrotate
        fail2ban-client flushlogs 1>/dev/null
    endscript

    # If fail2ban runs as non-root it still needs to have write access
    # to logfiles.
    # create 640 fail2ban adm
    create 640 root netdata
}
```

</details>

## Charts

- Failed attempts in attempts/s
- Bans in bans/s
- Banned IP addresses (since the last restart of netdata) in ips

## Configuration

Edit the `python.d/fail2ban.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/fail2ban.conf
```

Sample:

```yaml
local:
  log_path: '/var/log/fail2ban.log'
  conf_path: '/etc/fail2ban/jail.local'
  exclude: 'dropbear apache'
```

If no configuration is given, module will attempt to read log file at `/var/log/fail2ban.log` and conf file
at `/etc/fail2ban/jail.local`. If conf file is not found default jail is `ssh`.




### Troubleshooting

To troubleshoot issues with the `fail2ban` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `fail2ban` module in debug mode:

```bash
./python.d.plugin fail2ban debug trace
```

