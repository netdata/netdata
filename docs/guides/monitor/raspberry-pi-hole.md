<!--
title: "Monitor a Rasberry Pi (and Pi-hole) with Netdata"
description: "Start monitoring your Raspberry Pi's system metrics, including temperature sensors, plus services like Pi-Hole in a matter of minutes."
image: /img/seo/guides/monitor/raspberry-pi-hole.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/raspberry-pi-hole.md
-->

# Monitor a Rasberry Pi (and Pi-hole) with Netdata

intro t/k

## What's this guide about?

t/k

### What's a Raspberry Pi?

t/k

### What's Pi-hole?

t/k

### What's Netdata?

t/k

## Prerequisites

All you need to get started is a [Raspberry Pi](https://www.raspberrypi.org/products/raspberry-pi-4-model-b/) with
Raspbian installed. This guide uses a Raspberry Pi 4 Model B and Raspbian GNU/Linux 10 (buster).

This guide assumes you're connecting to a Raspberry Pi remotely over SSH, but you could also complete all these steps on
the system directly using a keyboard, mouse, and monitor.

## Install Netdata

Let's start by installing Netdata first. This way you'll start collecting system metrics as soon as possible for the
longest and richest database of historical metrics.

Because Rasbian is based on Debian, you can install Netdata using the one-line kickstart script. Check out our [install
docs](https://learn.netdata.cloud/docs/agent/packaging/installer/) if you want to try a different method.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

The installer asks you to install dependencies, then compiles Netdata from its source on
[GitHub](https://github.com/netdata/netdata).

Once installed, Netdata starts collecting roughly 1,500 metrics every second, and populates its dashboard with more than
250 charts.

Open your browser of choice and navigate to `http://NODE:19999/`, replacing `NODE` with the IP address of your Raspberry
Pi. Not sure what that IP is? Try running `hostname -I | awk '{print $1}'`. 

You should see a dashboard just like this:

**image of the dashboard**

Feel free to explore the dashboard straightaway, but 

## Install Pi-Hole

3. Run the Pi-Hole installer: `curl -sSL https://install.pi-hole.net | bash`
4. Run through the installer, set up based on your network
5. Set up your DNS
6. Restart Netdata
7. Go back to the dashbord, see Pi-hole metrics!

## Use Netdata to explore and monitor your Raspberry Pi

t/k

## Tweaks, configurations, and fun extras

### Enable temperature sensor monitoring

https://learn.netdata.cloud/docs/agent/netdata-for-iot/#monitoring-rpi-temperature

### Testing temperatures with and without a case fan

t/k

### Storing historical metrics on the Raspberry Pi

Optimizations? Performance?
Remove fan, see temperature before/after
DB engine sizing, storing X amount of metrics

### Performance optimizations

t/k

---

http://pi.hole/admin
http://192.168.1.10/admin
ae5xz-qe

https://docs.pi-hole.net/main/prerequisites/#supported-operating-systems
https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pihole

---

```bash
2020-07-19 07:12:12: netdata INFO  : PLUGIN[proc] : RRDSET: chart name 'ip.tcpof
o' on host 'raspberrypi' already exists.
2020-07-19 07:12:13: netdata INFO  : MAIN : SIGNAL: Received SIGUSR1. Saving dat
abases...
2020-07-19 07:12:13: netdata INFO  : MAIN : COMMAND: Saving databases.
2020-07-19 07:12:13: netdata INFO  : MAIN : Saving database [1 hosts(s)]...
2020-07-19 07:12:13: netdata INFO  : MAIN : Saving/Closing database of host 'raspberrypi'...
2020-07-19 07:12:13: netdata INFO  : MAIN : COMMAND: Databases saved.
2020-07-19 07:17:52: netdata INFO  : MAIN : SIGNAL: Received SIGTERM. Cleaning up to exit...
2020-07-19 07:17:52: netdata INFO  : MAIN : Shutting down command server.
2020-07-19 07:17:52: netdata INFO  : MAIN : Shutting down command event loop.
2020-07-19 07:17:52: netdata INFO  : MAIN : Shutting down command loop complete.
2020-07-19 07:17:52: netdata ERROR : PLUGIN[tc] : child pid 19235 killed by signal 15.
```

```bash
Jul 19 07:17:01 raspberrypi CRON[15226]: (root) CMD (   cd / && run-parts --repo
rt /etc/cron.hourly)
Jul 19 07:17:51 raspberrypi systemd[1]: Reloading.
Jul 19 07:17:51 raspberrypi systemd[1]: /lib/systemd/system/netdata.service:10: 
PIDFile= references path below legacy directory /var/run/, updating /var/run/net
data/netdata.pid → /run/netdata/netdata.pid; please update the unit file accordi
ngly.
Jul 19 07:17:51 raspberrypi systemd[1]: /lib/systemd/system/lighttpd.service:6: 
PIDFile= references path below legacy directory /var/run/, updating /var/run/lighttpd.pid → /run/lighttpd.pid; please update the unit file accordingly.
```
