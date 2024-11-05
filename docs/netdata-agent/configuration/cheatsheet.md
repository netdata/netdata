# Useful management and configuration actions

Below are some of the most common actions one can take while using Netdata. You can use this page as a quick reference for installing Netdata, connecting a node to the Cloud, properly editing the configuration, accessing Netdata's API, and more!

## Install Netdata

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh

# Or, if you have cURL but not wget (such as on macOS):
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh
```

### Connect a node to Netdata Cloud

To do so, sign in to Netdata Cloud, on your Space under the Nodes tab, click `Add Nodes` and paste the provided command into your node’s terminal and run it.
You can also copy the Claim token and pass it to the installation script with `--claim-token` and re-run it.

## Configuration

**Netdata's config directory** is `/etc/netdata/` but in some operating systems it might be `/opt/netdata/etc/netdata/`.  
Look for the `# config directory =` line over at `http://NODE_IP:19999/netdata.conf` to find your config directory.

From within that directory you can run `sudo ./edit-config netdata.conf` **to edit Netdata's configuration.**  
You can edit other config files too, by specifying their filename after `./edit-config`.  
You are expected to use this method in all following configuration changes.

### Enable/disable plugins (groups of collectors)

```bash
sudo ./edit-config netdata.conf
```

```text
[plugins]
 go.d = yes # enabled
 node.d = no # disabled
```

### Enable/disable specific collectors

```bash
sudo ./edit-config go.d.conf # edit a plugin's config
```

```yaml
modules:
  activemq: no # disabled
  cockroachdb: yes # enabled
```

### Edit a collector's config

```bash
sudo ./edit-config go.d/mysql.conf
```

## Alerts & notifications

After any change, reload the Netdata health configuration:

```bash
netdatacli reload-health
#or if that command doesn't work on your installation, use:
killall -USR2 netdata
```

### Configure a specific alert

```bash
sudo ./edit-config health.d/example-alert.conf
```

### Silence a specific alert

```bash
sudo ./edit-config health.d/example-alert.conf
```

```text
 to: silent
```

## Manage the daemon

| Intent                      |                                                      Action |
|:----------------------------|------------------------------------------------------------:|
| Start Netdata               |                              `$ sudo service netdata start` |
| Stop Netdata                |                               `$ sudo service netdata stop` |
| Restart Netdata             |                            `$ sudo service netdata restart` |
| Reload health configuration | `$ sudo netdatacli reload-health` `$ killall -USR2 netdata` |
| View error logs             |                           `less /var/log/netdata/error.log` |
| View collectors logs        |                       `less /var/log/netdata/collector.log` |

### Change the port Netdata listens to (example, set it to port 39999)

```text
[web]
default port = 39999
```

## See metrics and dashboards

### Netdata Cloud: `https://app.netdata.cloud`

### Local dashboard: `https://NODE:19999`

> Replace `NODE` with the IP address or hostname of your node. Often `localhost`.

## Access the Netdata API

You can access the API like this: `http://NODE:19999/api/VERSION/REQUEST`.  
If you want to take a look at all the API requests, check our API page at <https://learn.netdata.cloud/api>
