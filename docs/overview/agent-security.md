<!--
title: "Security design"
description: "Netdata has been designed with security in mind, running in read-only mode, without special privileges, and never exposing raw data."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/security-design.md
-->

# Security design

We have given special attention to all aspects of Netdata, ensuring that everything throughout its operation is as
secure as possible. Netdata has been designed with security in mind.

## Your data are safe with Netdata

Netdata collects raw data from many sources. For each source, Netdata uses a plugin that connects to the source (or reads the relative files produced by the source), receives raw data and processes them to calculate the metrics shown on Netdata dashboards.

Even if Netdata plugins connect to your database server, or read your application log file to collect raw data, the product of this data collection process is always a number of **chart metadata and metric values** (summarized data for dashboard visualization). All Netdata plugins (internal to the Netdata daemon, and external ones written in any computer language), convert raw data collected into metrics, and only these metrics are stored in Netdata databases, sent to upstream Netdata servers, or archived to external time-series databases.

> The **raw data** collected by Netdata, do not leave the host they are collected. **The only data Netdata exposes are chart metadata and metric values.**

This means that Netdata can safely be used in environments that require the highest level of data isolation (like PCI Level 1).

## Your systems are safe with Netdata

We are very proud that **the Netdata daemon runs as a normal system user, without any special privileges**. This is quite an achievement for a monitoring system that collects all kinds of system and application metrics.

There are a few cases however that raw source data are only exposed to processes with escalated privileges. To support these cases, Netdata attempts to minimize and completely isolate the code that runs with escalated privileges.

So, Netdata **plugins**, even those running with escalated capabilities or privileges, perform a **hard coded data collection job**. They do not accept commands from Netdata. The communication is strictly **unidirectional**: from the plugin towards the Netdata daemon. The original application data collected by each plugin do not leave the process they are collected, are not saved and are not transferred to the Netdata daemon. The communication from the plugins to the Netdata daemon includes only chart metadata and processed metric values.

Child nodes use the same protocol when streaming metrics to their parent nodes. The raw data collected by the plugins of
child Netdata servers are **never leaving the host they are collected**. The only data appearing on the wire are chart
metadata and metric values. This communication is also **unidirectional**: child nodes never accept commands from
parent Netdata servers.

## Netdata is read-only

Netdata **dashboards are read-only**. Dashboard users can view and examine metrics collected by Netdata, but cannot
instruct Netdata to do something other than present the already collected metrics.

Netdata dashboards do not expose sensitive information. Business data of any kind, the kernel version, O/S version,
application versions, host IPs, etc are not stored and are not exposed by Netdata on its dashboards.

## Netdata viewers authentication

Netdata is a monitoring system. It should be protected, the same way you protect all your admin apps. We assume Netdata
will be installed privately, for your eyes only.

### Why Netdata should be protected

Viewers will be able to get some information about the system Netdata is running. This information is everything the
dashboard provides. The dashboard includes a list of the services each system runs (the legends of the charts under the
`Systemd Services` section),  the applications running (the legends of the charts under the `Applications` section), the
disks of the system and their names, the user accounts of the system that are running processes (the `Users` and `User
Groups` section of the dashboard), the network interfaces and their names (not the IPs) and detailed information about
the performance of the system and its applications.

This information is not sensitive (meaning that it is not your business data), but **it is important for possible
attackers**. It will give them clues on what to check, what to try and in the case of DDoS against your applications,
they will know if they are doing it right or not.

Also, viewers could use Netdata itself to stress your servers. Although the Netdata daemon runs unprivileged, with the
minimum process priority (scheduling priority `idle` - lower than nice 19) and adjusts its OutOfMemory (OOM) score to
1000 (so that it will be first to be killed by the kernel if the system starves for memory), some pressure can be
applied on your systems if someone attempts a DDoS against Netdata.

See our document on [securing your nodes](/docs/configure/secure-nodes.md) for the most common strategies for protecting
the [dashboard](/docs/dashboard/how-dashboard-works.mdx).

Of course, there are many more methods you could use to protect Netdata:

- Bind Netdata to localhost and use `ssh -L 19998:127.0.0.1:19999 remote.netdata.ip` to forward connections of local
  port 19998 to remote port 19999. This way you can SSH to a Netdata server and then use `http://127.0.0.1:19998/` on
  your computer to access the remote Netdata dashboard.

- If you are always under a static IP, you can use the script given above to allow direct access to your Netdata servers
  without authentication, from all your static IPs.

- Install all your Netdata in **headless data collector** mode, forwarding all metrics in real-time to a parent Netdata
  server, which will be protected with authentication using an nginx server running locally at the parent Netdata
  server. This requires more resources (you will need a bigger parent Netdata server), but does not require any firewall
  changes, since all the child Netdata servers will not be listening for incoming connections.

## Netdata directories

| path|owner|permissions|Netdata|comments|
|:---|:----|:----------|:------|:-------|
| `/etc/netdata`|user `root`<br/>group `netdata`|dirs `0755`<br/>files `0640`|reads|**Netdata config files**<br/>may contain sensitive information, so group `netdata` is allowed to read them.|
| `/usr/libexec/netdata`|user `root`<br/>group `root`|executable by anyone<br/>dirs `0755`<br/>files `0644` or `0755`|executes|**Netdata plugins**<br/>permissions depend on the file - not all of them should have the executable flag.<br/>there are a few plugins that run with escalated privileges (Linux capabilities or `setuid`) - these plugins should be executable only by group `netdata`.|
| `/usr/share/netdata`|user `root`<br/>group `netdata`|readable by anyone<br/>dirs `0755`<br/>files `0644`|reads and sends over the network|**Netdata web static files**<br/>these files are sent over the network to anyone that has access to the Netdata web server. Netdata checks the ownership of these files (using settings at the `[web]` section of `netdata.conf`) and refuses to serve them if they are not properly owned. Symbolic links are not supported. Netdata also refuses to serve URLs with `..` in their name.|
| `/var/cache/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata ephemeral database files**<br/>Netdata stores its ephemeral real-time database here.|
| `/var/lib/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata permanent database files**<br/>Netdata stores here the registry data, health alarm log db, etc.|
| `/var/log/netdata`|user `netdata`<br/>group `root`|dirs `0755`<br/>files `0644`|writes, creates|**Netdata log files**<br/>all the Netdata applications, logs their errors or other informational messages to files in this directory. These files should be log rotated.|
