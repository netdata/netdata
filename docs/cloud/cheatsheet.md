# Useful management and configuration actions

Below you will find some of the most common actions that one can take while using Netdata. You can use this page as a quick reference for installing Netdata, connecting a node to the Cloud, properly editing the configuration, accessing Netdata's API, and more!

### Install Netdata

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh

# Or, if you have cURL but not wget (such as on macOS):
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh
```

#### Connect a node to Netdata Cloud

To do so, sign in to Netdata Cloud, on your Space under the Nodes tab, click `Add Nodes` and paste the provided command into your node’s terminal and run it.
You can also copy the Claim token and pass it to the installation script with `--claim-token` and re-run it.

### Configuration

**Netdata's config directory** is `/etc/netdata/` but in some operating systems it might be `/opt/netdata/etc/netdata/`.  
Look for the `# config directory =` line over at `http://NODE_IP:19999/netdata.conf` to find your config directory.

From within that directory you can run `sudo ./edit-config netdata.conf` **to edit Netdata's configuration.**  
You can edit other config files too, by specifying their filename after `./edit-config`.  
You are expected to use this method in all following configuration changes.

<!-- #### Edit Netdata's other config files (examples):

- `$ sudo ./edit-config apps_groups.conf`
- `$ sudo ./edit-config ebpf.conf`
- `$ sudo ./edit-config health.d/load.conf`
- `$ sudo ./edit-config go.d/prometheus.conf`

#### View the running Netdata configuration: `http://NODE:19999/netdata.conf`

> Replace `NODE` with the IP address or hostname of your node. Often `localhost`.

## Metrics collection & retention

You can tweak your settings in the netdata.conf file.
📄 [Find your netdata.conf file](https://github.com/netdata/netdata/blob/master/src/daemon/config/README.md)

Open a new terminal and navigate to the netdata.conf file. Use the edit-config script to make changes: `sudo ./edit-config netdata.conf`

The most popular settings to change are:

#### Increase metrics retention (4GiB)

```
sudo ./edit-config netdata.conf
```

```
[global]
 dbengine multihost disk space = 4096
```

#### Reduce the collection frequency (every 5 seconds)

```
sudo ./edit-config netdata.conf
```

```
[global]
 update every = 5
``` -->

---

#### Enable/disable plugins (groups of collectors)

```bash
sudo ./edit-config netdata.conf
```

```conf
[plugins]
 go.d = yes # enabled
 node.d = no # disabled
```

#### Enable/disable specific collectors

```bash
sudo ./edit-config go.d.conf # edit a plugin's config
```

```yaml
modules:
 activemq: no # disabled
 cockroachdb: yes # enabled
```

#### Edit a collector's config

```bash
sudo ./edit-config go.d/mysql.conf
```

### Alerts & notifications

<!-- #### Add a new alert

```
sudo touch health.d/example-alert.conf
sudo ./edit-config health.d/example-alert.conf
``` -->
After any change, reload the Netdata health configuration:

```bash
netdatacli reload-health
#or if that command doesn't work on your installation, use:
killall -USR2 netdata
```

#### Configure a specific alert

```bash
sudo ./edit-config health.d/example-alert.conf
```

#### Silence a specific alert

```bash
sudo ./edit-config health.d/example-alert.conf
```

```
 to: silent
```

<!-- #### Disable alerts and notifications

```conf
[health]
 enabled = no
``` -->

---

### Manage the daemon

| Intent                      |                                                      Action |
|:----------------------------|------------------------------------------------------------:|
| Start Netdata               |                              `$ sudo service netdata start` |
| Stop Netdata                |                               `$ sudo service netdata stop` |
| Restart Netdata             |                            `$ sudo service netdata restart` |
| Reload health configuration | `$ sudo netdatacli reload-health` `$ killall -USR2 netdata` |
| View error logs             |                           `less /var/log/netdata/error.log` |
| View collectors logs        |                       `less /var/log/netdata/collector.log` |

#### Change the port Netdata listens to (example, set it to port 39999)

```conf
[web]
default port = 39999
```

### See metrics and dashboards

#### Netdata Cloud: `https://app.netdata.cloud`

#### Local dashboard: `https://NODE:19999`

> Replace `NODE` with the IP address or hostname of your node. Often `localhost`.

### Access the Netdata API

You can access the API like this: `http://NODE:19999/api/VERSION/REQUEST`.  
If you want to take a look at all the API requests, check our API page at <https://learn.netdata.cloud/api>
<!-- 
## Interact with charts

| Intent                                 |                                                                                                                                                                                                                                                                          Action |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: |
| Stop a chart from updating             |                                                                                                                                                                                                                                                                         `click` |
| Zoom                                   | **Cloud** <br/> use the `zoom in` and `zoom out` buttons on any chart (upper right corner) <br/><br/> **Agent**<br/>`SHIFT` or `ALT` + `mouse scrollwheel` <br/> `SHIFT` or `ALT` + `two-finger pinch` (touchscreen) <br/> `SHIFT` or `ALT` + `two-finger scroll` (touchscreen) |
| Zoom to a specific timeframe           |                                                                                                                                **Cloud**<br/>use the `select and zoom` button on any chart and then do a `mouse selection` <br/><br/> **Agent**<br/>`SHIFT` + `mouse selection` |
| Pan forward or back in time            |                                                                                                                                                                                                                  `click` & `drag` <br/> `touch` & `drag` (touchpad/touchscreen) |
| Select a certain timeframe             |                                                                                                                                                                                `ALT` + `mouse selection` <br/> WIP need to evaluate this `command?` + `mouse selection` (macOS) |
| Reset to default auto refreshing state |                                                                                                                                                                                                                                                                  `double click` | -->

<!-- ## Dashboards

#### Disable the local dashboard

Use the `edit-config` script to edit the `netdata.conf` file.

```
[web]
mode = none
``` -->

<!-- #### Opt out from anonymous statistics

```
sudo touch .opt-out-from-anonymous-statistics
``` -->

<!-- ## Understanding the dashboard

**Charts**: A visualization displaying one or more collected/calculated metrics in a time series. Charts are generated
by collectors.

**Dimensions**: Any value shown on a chart, which can be raw or calculated values, such as percentages, averages,
minimums, maximums, and more.

**Families**: One instance of a monitored hardware or software resource that needs to be monitored and displayed
separately from similar instances. Example, disks named
**sda**, **sdb**, **sdc**, and so on.

**Contexts**: A grouping of charts based on the types of metrics collected and visualized.
**disk.io**, **disk.ops**, and **disk.backlog** are all contexts. -->
