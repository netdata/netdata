<!--
title: "MegaRAID controller monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/megacli/README.md"
sidebar_label: "MegaRAID controllers"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Devices"
-->

# MegaRAID controller collector

Collects adapter, physical drives and battery stats using `megacli` command-line tool.

Executed commands:

- `sudo -n megacli -LDPDInfo -aAll`
- `sudo -n megacli -AdpBbuCmd -a0`

## Requirements

The module uses `megacli`, which can only be executed by `root`. It uses
`sudo` and assumes that it is configured such that the `netdata` user can execute `megacli` as root without a password.

- Add to your `/etc/sudoers` file:

`which megacli` shows the full path to the binary.

```bash
netdata ALL=(root)       NOPASSWD: /path/to/megacli
```

- Reset Netdata's systemd
  unit [CapabilityBoundingSet](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities) (Linux
  distributions with systemd)

The default CapabilityBoundingSet doesn't allow using `sudo`, and is quite strict in general. Resetting is not optimal, but a next-best solution given the inability to execute `megacli` using `sudo`.


As the `root` user, do the following:

```cmd
mkdir /etc/systemd/system/netdata.service.d
echo -e '[Service]\nCapabilityBoundingSet=~' | tee /etc/systemd/system/netdata.service.d/unset-capability-bounding-set.conf
systemctl daemon-reload
systemctl restart netdata.service
```

## Charts

- Adapter State
- Physical Drives Media Errors
- Physical Drives Predictive Failures
- Battery Relative State of Charge
- Battery Cycle Count

## Enable the collector

The `megacli` collector is disabled by default. To enable it, use `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`, to edit the `python.d.conf`
file.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d.conf
```

Change the value of the `megacli` setting to `yes`. Save the file and restart the Netdata Agent
with `sudo systemctl restart netdata`, or the appropriate method for your system.

## Configuration

Edit the `python.d/megacli.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/megacli.conf
```

Battery stats disabled by default. To enable them, modify `megacli.conf`.

```yaml
do_battery: yes
```

Save the file and restart the Netdata Agent with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.


### Troubleshooting

To troubleshoot issues with the `megacli` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `megacli` module in debug mode:

```bash
./python.d.plugin megacli debug trace
```

