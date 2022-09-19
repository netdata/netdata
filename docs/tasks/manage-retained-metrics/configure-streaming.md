<!--
title: "Configure streaming"
sidebar_label: "Configure streaming"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/manage-retained-metrics/configure-streaming.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "manage-retained-metrics"
learn_docs_purpose: "Instructions on how to make a Parent-child setup"
-->

The simplest streaming configuration is **replication**, in which a child node streams its metrics in real time to a
parent node, and both nodes retain metrics in their own databases. You can read more in
our [Streaming Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
documentation.

## Prerequisites

To configure replication, you need two nodes, each of them running a Netdata instance.

## Steps

To begin, first you'll need to enable streaming on your "parent" node, then enable streaming on your "child" node.

### Enable streaming on the parent node

1. First, log onto the node that will act as the parent.

2. Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to identify
   itself while initiating a streaming connection. Copy that into a separate text file for later use.

   :::info
   Find out how to [install `uuidgen`](https://command-not-found.com/uuidgen) on your node if you don't already have it.
   :::

3. Next, open `stream.conf` using `edit-config` from within the Netdata config directory.  
   To read more about how to configure the Netdata Agent, check
   our [Agent Configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
   Task.

4. Scroll down to the section beginning with `[API_KEY]`. Paste the API key you generated earlier between the brackets,
   so that it looks like the following:

   ```conf
   [11111111-2222-3333-4444-555555555555]
   ```
5. Set `enabled` to `yes`, and `default memory mode` to `dbengine`. Leave all the other settings as their defaults. A
   simplified version of the configuration, minus the commented lines, looks like the following:

   ```conf
   [11111111-2222-3333-4444-555555555555]
       enabled = yes
       default memory mode = dbengine
   ```

6. Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the appropriate
   method for your system in
   the [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
   Task.

### Enable streaming on the child node

1. Connect via terminal to your child node with SSH.
2. Open `stream.conf` again.
3. Scroll down to the `[stream]` section and set `enabled` to `yes`.
4. Paste the IP address of your parent node at the end of the `destination` line
5. Paste the API key generated on the parent node onto the `api key` line.

   Leave all the other settings as their defaults. A simplified version of the configuration, minus the commented lines,
   looks like the following:

   ```conf
   [stream]
       enabled = yes 
       destination = 203.0.113.0
       api key = 11111111-2222-3333-4444-555555555555
   ```

6. Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the appropriate
   method for your system in
   the [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
   Task.

## Further actions

### Enable TLS/SSL on streaming (optional)

While encrypting the connection between your parent and child nodes is recommended for security, it's not required to
get started. If you're not interested in encryption, skip ahead to the [Expected result](#expected-result) section.

In this example, we'll use self-signed certificates.

1. On the **parent** node, use OpenSSL to create the key and certificate, then use `chown` to make the new files
   readable by the `netdata` user.

   ```bash
   sudo openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -keyout /etc/netdata/ssl/key.pem -out /etc/netdata/ssl/cert.pem
   sudo chown netdata:netdata /etc/netdata/ssl/cert.pem /etc/netdata/ssl/key.pem
   ```

2. Next, enforce TLS/SSL on the web server. Open `netdata.conf`, scroll down to the `[web]` section, and look for
   the `bind to` setting. Add `^SSL=force` to turn on TLS/SSL.

   See the [Netdata configuration Reference](https://github.com/netdata/netdata/blob/master/daemon/README.md) for other
   TLS/SSL options.

    ```conf
    [web]
       bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force
    ```

3. Next, connect to the **child** node and open `stream.conf`.   
   Add `:SSL` to the end of the existing `destination` setting to connect to the parent using TLS/SSL.   
   Uncomment the `ssl skip certificate verification` line to allow the use of self-signed certificates.

   ```conf
   [stream]
       enabled = yes
       destination = 203.0.113.0:SSL
       ssl skip certificate verification = yes
       api key = 11111111-2222-3333-4444-555555555555
   ```

4. Restart both the parent and child nodes with `sudo systemctl restart netdata`, or the [appropriate
   method](/docs/configure/start-stop-restart.md) for your system, to stream encrypted metrics using TLS/SSL.

## Expected result

At this point, the child node is streaming its metrics in real time to its parent. Open the local Agent dashboard for
the parent by navigating to `http://PARENT-NODE:19999` in your browser, replacing `PARENT-NODE` with its IP address or
hostname.

This dashboard shows parent metrics. To see child metrics, open the left-hand sidebar with the hamburger icon
![Hamburger icon](https://raw.githubusercontent.com/netdata/netdata-ui/master/src/components/icon/assets/hamburger.svg)
in the top panel. Both nodes appear under the **Replicated Nodes** menu. Click on either of the links to switch between
separate parent and child dashboards.

The child dashboard is also available directly at `http://PARENT-NODE:19999/host/CHILD-HOSTNAME`.

## Related topics

1. [Streaming Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
2. [Streaming Configuration Reference](https://github.com/netdata/netdata/blob/master/streaming/README.md)
