<!--
title: "Security design"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/netdata-security.md
sidebar_label: "Security Design"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Configuration"
sidebar_position: 20
-->

# Security design

We have given special attention to all aspects of Netdata, ensuring that everything throughout its operation is as secure as possible. Netdata has been designed with security in mind.

**Table of Contents**

- [Your data is safe with Netdata](#your-data-is-safe-with-netdata)
- [Your systems are safe with Netdata](#your-systems-are-safe-with-netdata)
- [Netdata is read-only](#netdata-is-read-only)
- [Why Netdata should be protected](#why-netdata-should-be-protected)
- [Protect Netdata from the internet](#protect-netdata-from-the-internet)
- [Anonymous Statistics](#anonymous-statistics)
- [Netdata directories](#netdata-directories)

## Your data is safe with Netdata

Netdata collects raw data from many sources. For each source, Netdata uses a plugin that connects to the source (or reads the relative files produced by the source), receives raw data and processes them to calculate the metrics shown on Netdata dashboards.

Even if Netdata plugins connect to your database server, or read your application log file to collect raw data, the product of this data collection process is always a number of **chart metadata and metric values** (summarized data for dashboard visualization). All Netdata plugins (internal to the Netdata daemon, and external ones written in any computer language), convert raw data collected into metrics, and only these metrics are stored in Netdata databases, sent to upstream Netdata servers, or archived to external time-series databases.

> The **raw data** collected by Netdata, does not leave the host when collected. **The only data Netdata exposes are chart metadata and metric values.**

This means that Netdata can safely be used in environments that require the highest level of data isolation (like PCI Level 1).

## Your systems are safe with Netdata

We are very proud that **the Netdata daemon runs as a normal system user, without any special privileges**. This is quite an achievement for a monitoring system that collects all kinds of system and application metrics.

There are a few cases, however, that raw source data are only exposed to processes with escalated privileges. To support these cases, Netdata attempts to minimize and completely isolate the code that runs with escalated privileges.

So, Netdata **plugins**, even those running with escalated capabilities or privileges, perform a **hard coded data collection job**. They do not accept commands from Netdata. The communication is strictly **unidirectional**: from the plugin towards the Netdata daemon. The original application data collected by each plugin do not leave the process they are collected, are not saved and are not transferred to the Netdata daemon. The communication from the plugins to the Netdata daemon includes only chart metadata and processed metric values.

Child nodes use the same protocol when streaming metrics to their parent nodes. The raw data collected by the plugins of
child Netdata servers are **never leaving the host they are collected**. The only data appearing on the wire are chart
metadata and metric values. This communication is also **unidirectional**: child nodes never accept commands from
parent Netdata servers.

## Netdata is read-only

Netdata **dashboards are read-only**. Dashboard users can view and examine metrics collected by Netdata, but cannot instruct Netdata to do something other than present the already collected metrics.

Netdata dashboards do not expose sensitive information. Business data of any kind, the kernel version, O/S version, application versions, host IPs, etc are not stored and are not exposed by Netdata on its dashboards.

## Why Netdata should be protected

Netdata is a monitoring system. It should be protected, the same way you protect all your admin apps. We assume Netdata will be installed privately, for your eyes only.

Upon installation, the Netdata Agent serves the **local dashboard** at port `19999`. If the node is accessible to the
internet at large, anyone can access the dashboard and your node's metrics at `http://NODE:19999`. We made this decision
so that the local dashboard was immediately accessible to users, and so that we don't dictate how professionals set up
and secure their infrastructures. 

Viewers will be able to get some information about the system Netdata is running. This information is everything the dashboard provides. The dashboard includes a list of the services each system runs (the legends of the charts under the `Systemd Services` section),  the applications running (the legends of the charts under the `Applications` section), the disks of the system and their names, the user accounts of the system that are running processes (the `Users` and `User Groups` section of the dashboard), the network interfaces and their names (not the IPs) and detailed information about the performance of the system and its applications.

This information is not sensitive (meaning that it is not your business data), but **it is important for possible attackers**. It will give them clues on what to check, what to try and in the case of DDoS against your applications, they will know if they are doing it right or not.

Also, viewers could use Netdata itself to stress your servers. Although the Netdata daemon runs unprivileged, with the minimum process priority (scheduling priority `idle` - lower than nice 19) and adjusts its OutOfMemory (OOM) score to 1000 (so that it will be first to be killed by the kernel if the system starves for memory), some pressure can be applied on your systems if someone attempts a DDoS against Netdata.

## Protect Netdata from the internet

Instead of dictating how to secure your infrastructure, we give you many options to establish security best practices
that align with your goals and your organization's standards.

- [Disable the local dashboard](#disable-the-local-dashboard): **Simplest and recommended method** for those who have
  added nodes to Netdata Cloud and view dashboards and metrics there.

- [Expose Netdata only in a private LAN](#expose-netdata-only-in-a-private-lan). Simplest and recommended method for those who do not use Netdata Cloud.

- [Fine-grained access control](#fine-grained-access-control): Allow local dashboard access from
  only certain IP addresses, such as a trusted static IP or connections from behind a management LAN. Full support for Netdata Cloud.

- [Use a reverse proxy (authenticating web server in proxy mode)](#use-an-authenticating-web-server-in-proxy-mode): Password-protect 
  a local dashboard and enable TLS to secure it. Full support for Netdata Cloud.

- [Other methods](#other-methods) list some less common methods of protecting Netdata.

### Disable the local dashboard

This is the _recommended method for those who have connected their nodes to Netdata Cloud_ and prefer viewing real-time
metrics using the War Room Overview, Nodes view, and Cloud dashboards.

You can disable the local dashboard (and API) but retain the encrypted Agent-Cloud link ([ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md)) that
allows you to stream metrics on demand from your nodes via the Netdata Cloud interface. This change mitigates all
concerns about revealing metrics and system design to the internet at large, while keeping all the functionality you
need to view metrics and troubleshoot issues with Netdata Cloud.

Open `netdata.conf` with `./edit-config netdata.conf`. Scroll down to the `[web]` section, and find the `mode =
static-threaded` setting, and change it to `none`.

```conf
[web]
    mode = none
```

Save and close the editor, then [restart your Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) using `sudo systemctl
restart netdata`. If you try to visit the local dashboard to `http://NODE:19999` again, the connection will fail because
that node no longer serves its local dashboard.

> See the [configuration basics doc](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) for details on how to find `netdata.conf` and use
> `edit-config`.

### Expose Netdata only in a private LAN

If your organisation has a private administration and management LAN, you can bind Netdata on this network interface on all your servers. This is done in `Netdata.conf` with these settings:

```
[web]
	bind to = 10.1.1.1:19999 localhost:19999
```

You can bind Netdata to multiple IPs and ports. If you use hostnames, Netdata will resolve them and use all the IPs (in the above example `localhost` usually resolves to both `127.0.0.1` and `::1`).

**This is the best and the suggested way to protect Netdata**. Your systems **should** have a private administration and management LAN, so that all management tasks are performed without any possibility of them being exposed on the internet.

For cloud based installations, if your cloud provider does not provide such a private LAN (or if you use multiple providers), you can create a virtual management and administration LAN with tools like `tincd` or `gvpe`. These tools create a mesh VPN allowing all servers to communicate securely and privately. Your administration stations join this mesh VPN to get access to management and administration tasks on all your cloud servers.

For `gvpe` we have developed a [simple provisioning tool](https://github.com/netdata/netdata-demo-site/tree/master/gvpe) you may find handy (it includes statically compiled `gvpe` binaries for Linux and FreeBSD, and also a script to compile `gvpe` on your macOS system). We use this to create a management and administration LAN for all Netdata demo sites (spread all over the internet using multiple hosting providers).

### Fine-grained access control

If you want to keep using the local dashboard, but don't want it exposed to the internet, you can restrict access with
[access lists](https://github.com/netdata/netdata/blob/master/web/server/README.md#access-lists). This method also fully retains the ability to stream metrics
on-demand through Netdata Cloud.

The `allow connections from` setting helps you allow only certain IP addresses or FQDN/hostnames, such as a trusted
static IP, only `localhost`, or connections from behind a management LAN. 

By default, this setting is `localhost *`. This setting allows connections from `localhost` in addition to _all_
connections, using the `*` wildcard. You can change this setting using Netdata's [simple
patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md).

```conf
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

The `allow connections from` setting is global and restricts access to the dashboard, badges, streaming, API, and
`netdata.conf`, but you can also set each of those access lists more granularly if you choose:

```conf
[web]
    allow connections from = localhost *
    allow dashboard from = localhost *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
    allow management from = localhost
```

See the [web server](https://github.com/netdata/netdata/blob/master/web/server/README.md#access-lists) docs for additional details about access lists. You can take
access lists one step further by [enabling SSL](https://github.com/netdata/netdata/blob/master/web/server/README.md#enabling-tls-support) to encrypt data from local
dashboard in transit. The connection to Netdata Cloud is always secured with TLS.

### Use an authenticating web server in proxy mode

Use one web server to provide authentication in front of **all your Netdata servers**. So, you will be accessing all your Netdata with URLs like `http://{HOST}/netdata/{NETDATA_HOSTNAME}/` and authentication will be shared among all of them (you will sign-in once for all your servers). Instructions are provided on how to set the proxy configuration to have Netdata run behind 
[nginx](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md), 
[HAproxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md), 
[Apache](https://github.com/netdata/netdata/blob/master/docs/Running-behind-apache.md), 
[lighthttpd](https://github.com/netdata/netdata/blob/master/docs/Running-behind-lighttpd.md), 
[caddy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-caddy.md), and
[H2O](https://github.com/netdata/netdata/blob/master/docs/Running-behind-h2o.md).

To use this method, you should firewall protect all your Netdata servers, so that only the web server IP will be allowed to directly access Netdata. To do this, run this on each of your servers (or use your firewall manager):

```sh
PROXY_IP="1.2.3.4"
iptables -t filter -I INPUT -p tcp --dport 19999 \! -s ${PROXY_IP} -m conntrack --ctstate NEW -j DROP
```

*commands to allow direct access to Netdata from a web server proxy*

The above will prevent anyone except your web server to access a Netdata dashboard running on the host.

For Netdata v1.9+ you can also use `netdata.conf`:

```
[web]
	allow connections from = localhost 1.2.3.4
```

Of course you can add more IPs.

For Netdata prior to v1.9, if you want to allow multiple IPs, use this:

```sh
# space separated list of IPs to allow access Netdata
NETDATA_ALLOWED="1.2.3.4 5.6.7.8 9.10.11.12"
NETDATA_PORT=19999

# create a new filtering chain || or empty an existing one named netdata
iptables -t filter -N netdata 2>/dev/null || iptables -t filter -F netdata
for x in ${NETDATA_ALLOWED}
do
	# allow this IP
    iptables -t filter -A netdata -s ${x} -j ACCEPT
done

# drop all other IPs
iptables -t filter -A netdata -j DROP

# delete the input chain hook (if it exists)
iptables -t filter -D INPUT -p tcp --dport ${NETDATA_PORT} -m conntrack --ctstate NEW -j netdata 2>/dev/null

# add the input chain hook (again)
# to send all new Netdata connections to our filtering chain
iptables -t filter -I INPUT -p tcp --dport ${NETDATA_PORT} -m conntrack --ctstate NEW -j netdata
```

_script to allow access to Netdata only from a number of hosts_

You can run the above any number of times. Each time it runs it refreshes the list of allowed hosts.

### Other methods

Of course, there are many more methods you could use to protect Netdata:

-   Bind Netdata to localhost and use `ssh -L 19998:127.0.0.1:19999 remote.netdata.ip` to forward connections of local port 19998 to remote port 19999. This way you can ssh to a Netdata server and then use `http://127.0.0.1:19998/` on your computer to access the remote Netdata dashboard.

-   If you are always under a static IP, you can use the script given above to allow direct access to your Netdata servers without authentication, from all your static IPs.

-   Install all your Netdata in **headless data collector** mode, forwarding all metrics in real-time to a parent
    Netdata server, which will be protected with authentication using an nginx server running locally at the parent
    Netdata server. This requires more resources (you will need a bigger parent Netdata server), but does not require
    any firewall changes, since all the child Netdata servers will not be listening for incoming connections.

## Anonymous Statistics

### Registry or how to not send any information to a third party server

The default configuration uses a public registry under registry.my-netdata.io (more information about the registry here: [mynetdata-menu-item](https://github.com/netdata/netdata/blob/master/registry/README.md) ). Please be aware that if you use that public registry, you submit the following information to a third party server: 

-   The url where you open the web-ui in the browser (via http request referrer)
-   The hostnames of the Netdata servers

If sending this information to the central Netdata registry violates your security policies, you can configure Netdata to [run your own registry](https://github.com/netdata/netdata/blob/master/registry/README.md#run-your-own-registry).

### Opt-out of anonymous statistics

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self hosted PostHog instance within the Netdata infrastructure. Read
about the information collected, and learn how to-opt, on our [anonymous statistics](anonymous-statistics.md) page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

## Netdata directories

| path|owner|permissions|Netdata|comments|
|:---|:----|:----------|:------|:-------|
| `/etc/netdata`|user `root`<br/>group `netdata`|dirs `0755`<br/>files `0640`|reads|**Netdata config files**<br/>may contain sensitive information, so group `netdata` is allowed to read them.|
| `/usr/libexec/netdata`|user `root`<br/>group `root`|executable by anyone<br/>dirs `0755`<br/>files `0644` or `0755`|executes|**Netdata plugins**<br/>permissions depend on the file - not all of them should have the executable flag.<br/>there are a few plugins that run with escalated privileges (Linux capabilities or `setuid`) - these plugins should be executable only by group `netdata`.|
| `/usr/share/netdata`|user `root`<br/>group `netdata`|readable by anyone<br/>dirs `0755`<br/>files `0644`|reads and sends over the network|**Netdata web static files**<br/>these files are sent over the network to anyone that has access to the Netdata web server. Netdata checks the ownership of these files (using settings at the `[web]` section of `netdata.conf`) and refuses to serve them if they are not properly owned. Symbolic links are not supported. Netdata also refuses to serve URLs with `..` in their name.|
| `/var/cache/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata ephemeral database files**<br/>Netdata stores its ephemeral real-time database here.|
| `/var/lib/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata permanent database files**<br/>Netdata stores here the registry data, health alarm log db, etc.|
| `/var/log/netdata`|user `netdata`<br/>group `root`|dirs `0755`<br/>files `0644`|writes, creates|**Netdata log files**<br/>all the Netdata applications, logs their errors or other informational messages to files in this directory. These files should be log rotated.|


