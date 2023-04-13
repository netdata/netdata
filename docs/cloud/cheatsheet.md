# Management and configuration cheatsheet

import {
	OneLineInstallWget,
	OneLineInstallCurl,
} from '@site/src/components/OneLineInstall/';

Use our management &amp; configuration cheatsheet to simplify your interactions with Netdata, including configuration,
using charts, managing the daemon, and more.

## Install Netdata

#### Install Netdata

<OneLineInstallWget />

Or, if you have cURL but not wget (such as on macOS):

<OneLineInstallCurl />

#### Claim a node to Netdata Cloud

To do so, sign in to Netdata Cloud, click the `Claim Nodes` button, choose the `War Rooms` to add nodes to, then click `Copy` to copy the full script to your clipboard. Paste that into your node’s terminal and run it.

## Metrics collection & retention

You can tweak your settings in the netdata.conf file.
📄 [Find your netdata.conf file](https://github.com/netdata/netdata/blob/master/daemon/config/README.md)

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
```

#### Enable/disable plugins (groups of collectors)

```
sudo ./edit-config netdata.conf
```

```
[plugins]
 go.d = yes # enabled
 node.d = no # disabled
```

#### Enable/disable specific collectors

```
sudo ./edit-config go.d.conf
```

> `Or python.d.conf, node.d.conf, edbpf.conf, and so on`.

```
modules:
 activemq: no # disabled
 bind: no # disabled
 cockroachdb: yes # enabled
```

#### Edit a collector's config (example)

```
$ sudo ./edit-config go.d/mysql.conf
$ sudo ./edit-config ebpf.conf
$ sudo ./edit-config python.d/anomalies.conf
```

## Configuration

#### The Netdata config directory: `/etc/netdata`

> If you don't have such a directory:
> 📄 [Find your netdata.conf file](https://github.com/netdata/netdata/blob/master/daemon/config/README.md)
> The cheatsheet assumes you’re running all commands from within the Netdata config directory!

#### Edit Netdata's main config file: `$ sudo ./edit-config netdata.conf`

#### Edit Netdata's other config files (examples):

- `$ sudo ./edit-config apps_groups.conf`
- `$ sudo ./edit-config ebpf.conf`
- `$ sudo ./edit-config health.d/load.conf`
- `$ sudo ./edit-config go.d/prometheus.conf`

#### View the running Netdata configuration: `http://NODE:19999/netdata.conf`

> Replace `NODE` with the IP address or hostname of your node. Often `localhost`.

## Alarms & notifications

#### Add a new alarm

```
sudo touch health.d/example-alarm.conf
sudo ./edit-config health.d/example-alarm.conf
```

#### Configure a specific alarm

```
sudo ./edit-config health.d/example-alarm.conf
```

#### Silence a specific alarm

```
sudo ./edit-config health.d/example-alarm.conf
 to: silent
```

#### Disable alarms and notifications

```
[health]
 enabled = no
```

> After any change, reload the Netdata health configuration

```
netdatacli reload-health
```

or if that command doesn't work on your installation, use:

```
killall -USR2 netdata
```

## Manage the daemon

| Intent                      |                                                                Action |
| :-------------------------- | --------------------------------------------------------------------: |
| Start Netdata               |                                      `$ sudo systemctl start netdata` |
| Stop Netdata                |                                       `$ sudo systemctl stop netdata` |
| Restart Netdata             |                                    `$ sudo systemctl restart netdata` |
| Reload health configuration | `$ sudo netdatacli reload-health` <br></br> `$ killall -USR2 netdata` |
| View error logs             |                                     `less /var/log/netdata/error.log` |

## See metrics and dashboards

#### Netdata Cloud: `https://app.netdata.cloud`

#### Local dashboard: `https://NODE:19999`

> Replace `NODE` with the IP address or hostname of your node. Often `localhost`.

#### Access the Netdata API: `http://NODE:19999/api/v1/info`

## Interact with charts

| Intent                                 |                                                                                                                                                                                                                                                                          Action |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: |
| Stop a chart from updating             |                                                                                                                                                                                                                                                                         `click` |
| Zoom                                   | **Cloud** <br/> use the `zoom in` and `zoom out` buttons on any chart (upper right corner) <br/><br/> **Agent**<br/>`SHIFT` or `ALT` + `mouse scrollwheel` <br/> `SHIFT` or `ALT` + `two-finger pinch` (touchscreen) <br/> `SHIFT` or `ALT` + `two-finger scroll` (touchscreen) |
| Zoom to a specific timeframe           |                                                                                                                                **Cloud**<br/>use the `select and zoom` button on any chart and then do a `mouse selection` <br/><br/> **Agent**<br/>`SHIFT` + `mouse selection` |
| Pan forward or back in time            |                                                                                                                                                                                                                  `click` & `drag` <br/> `touch` & `drag` (touchpad/touchscreen) |
| Select a certain timeframe             |                                                                                                                                                                                `ALT` + `mouse selection` <br/> WIP need to evaluate this `command?` + `mouse selection` (macOS) |
| Reset to default auto refreshing state |                                                                                                                                                                                                                                                                  `double click` |

## Dashboards

#### Disable the local dashboard

Use the `edit-config` script to edit the `netdata.conf` file.

```
[web]
mode = none
```

#### Change the port Netdata listens to (port 39999)

```
[web]
default port = 39999
```

#### Opt out from anonymous statistics

```
sudo touch .opt-out-from-anonymous-statistics
```

## Understanding the dashboard

**Charts**: A visualization displaying one or more collected/calculated metrics in a time series. Charts are generated
by collectors.

**Dimensions**: Any value shown on a chart, which can be raw or calculated values, such as percentages, averages,
minimums, maximums, and more.

**Families**: One instance of a monitored hardware or software resource that needs to be monitored and displayed
separately from similar instances. Example, disks named
**sda**, **sdb**, **sdc**, and so on.

**Contexts**: A grouping of charts based on the types of metrics collected and visualized.
**disk.io**, **disk.ops**, and **disk.backlog** are all contexts.
