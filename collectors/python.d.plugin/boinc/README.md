<!--
title: "BOINC monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/boinc/README.md"
sidebar_label: "BOINC"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Distributed computing"
-->

# BOINC collector

Monitors task counts for the Berkeley Open Infrastructure Networking Computing (BOINC) distributed computing client using the same RPC interface that the BOINC monitoring GUI does.

It provides charts tracking the total number of tasks and active tasks, as well as ones tracking each of the possible states for tasks.

## Configuration

Edit the `python.d/boinc.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/boinc.conf
```

BOINC requires use of a password to access it's RPC interface.  You can
find this password in the `gui_rpc_auth.cfg` file in your BOINC directory.

By default, the module will try to auto-detect the password by looking
in `/var/lib/boinc` for this file (this is the location most Linux
distributions use for a system-wide BOINC installation), so things may
just work without needing configuration for the local system.

You can monitor remote systems as well:

```yaml
remote:
  hostname: some-host
  password: some-password
```




### Troubleshooting

To troubleshoot issues with the `boinc` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `boinc` module in debug mode:

```bash
./python.d.plugin boinc debug trace
```

