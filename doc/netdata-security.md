# Netdata Security

We have given special attention to all aspects of netdata, ensuring that everything throughout its operation is as secure as possible. netdata has been designed with security in mind.

**Table of Contents**

1. [your data are safe with netdata](#your-data-are-safe-with-netdata)
2. [your systems are safe with netdata](#your-systems-are-safe-with-netdata)
3. [netdata is read-only](#netdata-is-read-only)
4. [netdata viewers authentication](#netdata-viewers-authentication)
	- [why netdata should be protected](#why-netdata-should-be-protected)
	- [protect netdata from the internet](#protect-netdata-from-the-internet)
		- [expose netdata only in a private LAN](#expose-netdata-only-in-a-private-lan)
		- [use an authenticating web server in proxy mode](#use-an-authenticating-web-server-in-proxy-mode)
		- [other methods](#other-methods)
5. [registry or how to not send any information to a third party server](#registry-or-how-to-not-send-any-information-to-a-third-party-server)

## your data are safe with netdata

netdata collects raw data from many sources. For each source, netdata uses a plugin that connects to the source (or reads the relative files produced by the source), receives raw data and processes them to calculate the metrics shown on netdata dashboards.

Even if netdata plugins connect to your database server, or read your application log file to collect raw data, the product of this data collection process is always a number of **chart metadata and metric values** (summarized data for dashboard visualization). All netdata plugins (internal to the netdata daemon, and external ones written in any computer language), convert raw data collected into metrics, and only these metrics are stored in netdata databases, sent to upstream netdata servers, or archived to backend time-series databases.

> The **raw data** collected by netdata, do not leave the host they are collected. **The only data netdata exposes are chart metadata and metric values.**

This means that netdata can safely be used in environments that require the highest level of data isolation (like PCI Level 1).

## your systems are safe with netdata

We are very proud that **the netdata daemon runs as a normal system user, without any special privileges**. This is quite an achievement for a monitoring system that collects all kinds of system and application metrics.

There are a few cases however that raw source data are only exposed to processes with escalated privileges. To support these cases, netdata attempts to minimize and completely isolate the code that runs with escalated privileges.

So, netdata **plugins**, even those running with escalated capabilities or privileges, perform a **hard coded data collection job**. They do not accept commands from netdata. The communication is strictly **unidirectional**: from the plugin towards the netdata daemon. The original application data collected by each plugin do not leave the process they are collected, are not saved and are not transferred to the netdata daemon. The communication from the plugins to the netdata daemon includes only chart metadata and processed metric values.

netdata slaves streaming metrics to upstream netdata servers, use exactly the same protocol local plugins use. The raw data collected by the plugins of slave netdata servers are **never leaving the host they are collected**. The only data appearing on the wire are chart metadata and metric values. This communication is also **unidirectional**: slave netdata servers never accept commands from master netdata servers.

## netdata is read-only

netdata **dashboards are read-only**. Dashboard users can view and examine metrics collected by netdata, but cannot instruct netdata to do something other than present the already collected metrics.

netdata dashboards do not expose sensitive information. Business data of any kind, the kernel version, O/S version, application versions, host IPs, etc are not stored and are not exposed by netdata on its dashboards.

## netdata viewers authentication

netdata is a monitoring system. It should be protected, the same way you protect all your admin apps. We assume netdata will be installed privately, for your eyes only.

### why netdata should be protected

Viewers will be able to get some information about the system netdata is running. This information is everything the dashboard provides. The dashboard includes a list of the services each system runs (the legends of the charts under the `Systemd Services` section),  the applications running (the legends of the charts under the `Applications` section), the disks of the system and their names, the user accounts of the system that are running processes (the `Users` and `User Groups` section of the dashboard), the network interfaces and their names (not the IPs) and detailed information about the performance of the system and its applications.

This information is not sensitive (meaning that it is not your business data), but **it is important for possible attackers**. It will give them clues on what to check, what to try and in the case of DDoS against your applications, they will know if they are doing it right or not.

Also, viewers could use netdata itself to stress your servers. Although the netdata daemon runs unprivileged, with the minimum process priority (scheduling priority `idle` - lower than nice 19) and adjusts its OutOfMemory (OOM) score to 1000 (so that it will be first to be killed by the kernel if the system starves for memory), some pressure can be applied on your systems if someone attempts a DDoS against netdata.

### protect netdata from the internet

netdata is a distributed application. Most likely you will have many installations of it. Since it is distributed and you are expected to jump from server to server, there is very little usability to add authentication local on each netdata.

Until we add a distributed authentication method to netdata, you have the following options:

#### expose netdata only in a private LAN

If your organisation has a private administration and management LAN, you can bind netdata on this network interface on all your servers. This is done in `netdata.conf` with these settings:

```
[web]
	bind to = 10.1.1.1:19999 localhost:19999
```

You can bind netdata to multiple IPs and ports. If you use hostnames, netdata will resolve them and use all the IPs (in the above example `localhost` usually resolves to both `127.0.0.1` and `::1`).

**This is the best and the suggested way to protect netdata**. Your systems **should** have a private administration and management LAN, so that all management tasks are performed without any possibility of them being exposed on the internet.

For cloud based installations, if your cloud provider does not provide such a private LAN (or if you use multiple providers), you can create a virtual management and administration LAN with tools like `tincd` or `gvpe`. These tools create a mesh VPN allowing all servers to communicate securely and privately. Your administration stations join this mesh VPN to get access to management and administration tasks on all your cloud servers.

For `gvpe` we have developed a [simple provisioning tool](https://github.com/netdata/netdata-demo-site/tree/master/gvpe) you may find handy (it includes statically compiled `gvpe` binaries for Linux and FreeBSD, and also a script to compile `gvpe` on your Mac). We use this to create a management and administration LAN for all netdata demo sites (spread all over the internet using multiple hosting providers).

---

In netdata v1.9+ there is also access list support, like this:

```
[web]
	bind to = *
	allow connections from = localhost 10.* 192.168.*
```


#### use an authenticating web server in proxy mode

Use **one nginx** (or one apache) server to provide authentication in front of **all your netdata servers**. So, you will be accessing all your netdata with URLs like `http://nginx.host/netdata/{NETDATA_HOSTNAME}/` and authentication will be shared among all of them (you will sign-in once for all your servers). Check [this wiki page for more information on configuring nginx for such a setup](https://github.com/netdata/netdata/blob/master/doc/Running-behind-nginx.md#netdata-via-nginx).

To use this method, you should firewall protect all your netdata servers, so that only the nginx IP will allowed to directly access netdata. To do this, run this on each of your servers (or use your firewall manager):

```sh
NGINX_IP="1.2.3.4"
iptables -t filter -I INPUT -p tcp --dport 19999 \! -s ${NGINX_IP} -m conntrack --ctstate NEW -j DROP
```
_commands to allow direct access to netdata from an nginx proxy_

The above will prevent anyone except your nginx server to access a netdata dashboard running on the host.

For netdata v1.9+ you can also use `netdata.conf`:

```
[web]
	allow connections from = localhost 1.2.3.4
```

Of course you can add more IPs.

For netdata prior to v1.9, if you want to allow multiple IPs, use this:

```sh
# space separated list of IPs to allow access netdata
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
# to send all new netdata connections to our filtering chain
iptables -t filter -I INPUT -p tcp --dport ${NETDATA_PORT} -m conntrack --ctstate NEW -j netdata
```
_script to allow access to netdata only from a number of hosts_

You can run the above any number of times. Each time it runs it refreshes the list of allowed hosts.

#### other methods

Of course, there are many more methods you could use to protect netdata:

- bind netdata to localhost and use `ssh -L 19998:127.0.0.1:19999 remote.netdata.ip` to forward connections of local port 19998 to remote port 19999. This way you can ssh to a netdata server and then use `http://127.0.0.1:19998/` on your computer to access the remote netdata dashboard.

- If you are always under a static IP, you can use the script given above to allow direct access to your netdata servers without authentication, from all your static IPs.

- install all your netdata in **headless data collector** mode, forwarding all metrics in real-time to a master netdata server, which will be protected with authentication using an nginx server running locally at the master netdata server. This requires more resources (you will need a bigger master netdata server), but does not require any firewall changes, since all the slave netdata servers will not be listening for incoming connections.

## registry or how to not send any information to a third party server

The default configuration uses a public registry under registry.my-netdata.io (more information about the registry here: [mynetdata-menu-item](https://github.com/netdata/netdata/tree/master/registry) ). Please be aware that if you use that public registry, you submit at least the following information to a third party server, which might violate your security policies: 
- Your public ip where the browser runs
- The url where you open the web-ui in the browser (via http request referer)
- The hostnames of the netdata servers

You are able to run your own registry, which is pretty simple to do:
- If you have just one netdata web-ui, turn on registry and set the url of that web-ui as "registry to announce"
```
[registry]
enabled = yes
registry to announce = URL_OF_THE_NETDATA_WEB-UI
```
- If you run multiple netdata servers with web-ui, you need to define one as registry. On that node activate the registry and setting its url as "registry to announce". On all other nodes do not enable the registry but define the same url.

restart netdata and check with developer tools of your browser which registry is called.

## netdata directories

path|owner|permissions| netdata |comments|
:---|:----|:----------|:--------|:-------|
`/etc/netdata`|user&nbsp;`root`<br/>group&nbsp;`netdata`|dirs `0755`<br/>files `0640`|reads|**netdata config files**<br/>may contain sensitive information, so group `netdata` is allowed to read them.
`/usr/libexec/netdata`|user&nbsp;`root`<br/>group&nbsp;`root`|executable by anyone<br/>dirs `0755`<br/>files `0644` or `0755`|executes|**netdata plugins**<br/>permissions depend on the file - not all of them should have the executable flag.<br/>there are a few plugins that run with escalated privileges (Linux capabilities or `setuid`) - these plugins should be executable only by group `netdata`.
`/usr/share/netdata`|user&nbsp;`root`<br/>group&nbsp;`netdata`|readable by anyone<br/>dirs `0755`<br/>files `0644`|reads and sends over the network|**netdata web static files**<br/>these files are sent over the network to anyone that has access to the netdata web server. netdata checks the ownership of these files (using settings at the `[web]` section of `netdata.conf`) and refuses to serve them if they are not properly owned. Symbolic links are not supported. netdata also refuses to serve URLs with `..` in their name.
`/var/cache/netdata`|user&nbsp;`netdata`<br/>group&nbsp;`netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**netdata ephemeral database files**<br/>netdata stores its ephemeral real-time database here.
`/var/lib/netdata`|user&nbsp;`netdata`<br/>group&nbsp;`netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**netdata permanent database files**<br/>netdata stores here the registry data, health alarm log db, etc.
`/var/log/netdata`|user&nbsp;`netdata`<br/>group&nbsp;`root`|dirs `0755`<br/>files `0644`|writes, creates|**netdata log files**<br/>all the netdata applications, logs their errors or other informational messages to files in this directory. These files should be log rotated.
