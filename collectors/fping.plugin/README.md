# fping.plugin

The fping plugin supports monitoring latency, packet loss and uptime of any number of network end points,
by pinging them with `fping`.

A recent version of `fping` is required (one that supports option ` -N `).
The supplied plugin can install it, by running:

```sh
/usr/libexec/netdata/plugins.d/fping.plugin install
```

The above will download, build and install the right version as `/usr/local/bin/fping`.

Then you need to edit `/etc/netdata/fping.conf` (to edit it on your system run
`/etc/netdata/edit-config fping.conf`) like this:

```sh
# uncomment the following line - it should already be there
fping="/usr/local/bin/fping"

# set here all the hosts you need to ping
# I suggest to use hostnames and put their IPs in /etc/hosts
hosts="host1 host2 host3"

# override the chart update frequency - the default is inherited from netdata
update_every=1

# time in milliseconds (1 sec = 1000 ms) to ping the hosts
# 200 = 5 pings per second
ping_every=200

# other fping options - these are the defaults
fping_opts="-R -b 56 -i 1 -r 0 -t 5000"
```

## alarms

netdata will automatically attach a few alarms for each host.
Check the [latest versions of the fping alarms](../../health/health.d/fping.conf)

## Additional Tips 

### Customizing Amount of Pings Per Second

For example, to update the chart every 10 seconds and use 2 pings every 10 seconds, use this:

```sh
# Chart Update Frequency (Time in Seconds)
update_every=10

# Time in Milliseconds (1 sec = 1000 ms) to Ping the Hosts
# The Following Example Sends 1 Ping Every 5000 ms
# Calculation Formula: ping_every = (update_every * 1000 ) / 2
ping_every=5000
```

### Multiple fping Plugins With Different Settings

You may need to run multiple fping plugins with different settings for different end points.
For example, you may need to ping a few hosts 10 times per second, and others once per second.

netdata allows you to add as many `fping` plugins as you like.

Follow this procedure:

**1. Create New fping Configuration File**


```sh
# Step Into Configuration Directory
cd /etc/netdata

# Copy Original fping Configuration File To New Configuration File
cp fping.conf fping2.conf
```

Edit `fping2.conf` and set the settings and the hosts you need for the seconds instance.

**2. Soft Link Original fping Plugin to New Plugin File**

```sh
# Become root (If The Step Step Is Performed As Non-Root User)
sudo su

# Step Into The Plugins Directory
cd /usr/libexec/netdata/plugins.d

# Link fping.plugin to fping2.plugin
ln -s fping.plugin fping2.plugin
```

That's it. netdata will detect the new plugin and start it.

You can name the new plugin any name you like.
Just make sure the plugin and the configuration file have the same name.
