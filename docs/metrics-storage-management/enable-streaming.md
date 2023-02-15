<!--
title: "Enable streaming between nodes"
description: >-
    "With metrics streaming enabled, you can not only replicate metrics data 
    into a second database, but also view dashboards and trigger alarm notifications 
    for multiple nodes in parallel."
type: "how-to"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md"
sidebar_label: "Enable streaming between nodes"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
-->

# Enable streaming between nodes

The simplest streaming configuration is **replication**, in which a child node streams its metrics in real time to a
parent node, and both nodes retain metrics in their own databases.

To configure replication, you need two nodes, each running Netdata. First you'll first enable streaming on your parent
node, then enable streaming on your child node. When you're finished, you'll be able to see the child node's metrics in
the parent node's dashboard, quickly switch between the two dashboards, and be able to serve [alarm
notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) from either or both nodes.

## Enable streaming on the parent node

First, log onto the node that will act as the parent.

Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to identify
itself while initiating a streaming connection. Copy that into a separate text file for later use.

> Find out how to [install `uuidgen`](https://command-not-found.com/uuidgen) on your node if you don't already have it.

Next, open `stream.conf` using [`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files)
from within the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata
sudo ./edit-config stream.conf
```

Scroll down to the section beginning with `[API_KEY]`. Paste the API key you generated earlier between the brackets, so
that it looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
```

Set `enabled` to `yes`, and `default memory mode` to `dbengine`. Leave all the other settings as their defaults. A
simplified version of the configuration, minus the commented lines, looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
    enabled = yes
    default memory mode = dbengine
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

## Enable streaming on the child node

Connect to your child node with SSH.

Open `stream.conf` again. Scroll down to the `[stream]` section and set `enabled` to `yes`. Paste the IP address of your
parent node at the end of the `destination` line, and paste the API key generated on the parent node onto the `api key`
line.

Leave all the other settings as their defaults. A simplified version of the configuration, minus the commented lines,
looks like the following:

```conf
[stream]
    enabled = yes 
    destination = 203.0.113.0
    api key = 11111111-2222-3333-4444-555555555555
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

## Enable TLS/SSL on streaming (optional)

While encrypting the connection between your parent and child nodes is recommended for security, it's not required to
get started. If you're not interested in encryption, skip ahead to [view streamed
metrics](#view-streamed-metrics-in-netdatas-dashboard).

In this example, we'll use self-signed certificates. 

On the **parent** node, use OpenSSL to create the key and certificate, then use `chown` to make the new files readable
by the `netdata` user.

```bash
sudo openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -keyout /etc/netdata/ssl/key.pem -out /etc/netdata/ssl/cert.pem
sudo chown netdata:netdata /etc/netdata/ssl/cert.pem /etc/netdata/ssl/key.pem
```

Next, enforce TLS/SSL on the web server. Open `netdata.conf`, scroll down to the `[web]` section, and look for the `bind
to` setting. Add `^SSL=force` to turn on TLS/SSL. See the [web server
reference](https://github.com/netdata/netdata/blob/master/web/server/README.md#enabling-tls-support) for other TLS/SSL options.

```conf
[web]
    bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force
```

Next, connect to the **child** node and open `stream.conf`. Add `:SSL` to the end of the existing `destination` setting
to connect to the parent using TLS/SSL. Uncomment the `ssl skip certificate verification` line to allow the use of
self-signed certificates.

```conf
[stream]
    enabled = yes
    destination = 203.0.113.0:SSL
    ssl skip certificate verification = yes
    api key = 11111111-2222-3333-4444-555555555555
```

Restart both the parent and child nodes with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system, to stream encrypted metrics using TLS/SSL.

## View streamed metrics in Netdata's dashboard

At this point, the child node is streaming its metrics in real time to its parent. Open the local Agent dashboard for
the parent by navigating to `http://PARENT-NODE:19999` in your browser, replacing `PARENT-NODE` with its IP address or
hostname.

This dashboard shows parent metrics. To see child metrics, open the left-hand sidebar with the hamburger icon
![Hamburger icon](https://raw.githubusercontent.com/netdata/netdata-ui/master/src/components/icon/assets/hamburger.svg)
in the top panel. Both nodes appear under the **Replicated Nodes** menu. Click on either of the links to switch between
separate parent and child dashboards.

![Switching between parent and child
dashboards](https://user-images.githubusercontent.com/1153921/110043346-761ec000-7d04-11eb-8e58-77670ba39161.gif)

The child dashboard is also available directly at `http://PARENT-NODE:19999/host/CHILD-HOSTNAME`, which in this example
is `http://203.0.113.0:19999/host/netdata-child`.

## What's next?

Now that you have a basic streaming setup with replication, you may want to tweak the configuration to eliminate the
child database, disable the child dashboard, or enable SSL on the streaming connection between the parent and child.

See the [streaming reference
doc](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/reference-streaming.md#examples) for details about
other possible configurations.

When using Netdata's default TSDB (`dbengine`), the parent node maintains separate, parallel databases for itself and
every child node streaming to it. Each instance is sized identically based on the `dbengine multihost disk space`
setting in `netdata.conf`. See our doc on [changing metrics retention](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md) for
details.

### Related information & further reading

- Streaming
  - [How Netdata streams metrics](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/how-streaming-works.md)
  - **[Enable streaming between nodes](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md)**
  - [Streaming reference](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/reference-streaming.md)
