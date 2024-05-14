# Securing Netdata Agents

Netdata is a monitoring system. It should be protected, the same way you protect all your admin apps. We assume Netdata 
will be installed privately, for your eyes only.

Upon installation, the Netdata Agent serves the **local dashboard** at port `19999`. If the node is accessible to the
internet at large, anyone can access the dashboard and your node's metrics at `http://NODE:19999`. We made this decision
so that the local dashboard was immediately accessible to users, and so that we don't dictate how professionals set up
and secure their infrastructures. 

Viewers will be able to get some information about the system Netdata is running. This information is everything the dashboard 
provides. The dashboard includes a list of the services each system runs (the legends of the charts under the `Systemd Services` 
section),  the applications running (the legends of the charts under the `Applications` section), the disks of the system and 
their names, the user accounts of the system that are running processes (the `Users` and `User Groups` section of the dashboard), 
the network interfaces and their names (not the IPs) and detailed information about the performance of the system and its applications.

This information is not sensitive (meaning that it is not your business data), but **it is important for possible attackers**. 
It will give them clues on what to check, what to try and in the case of DDoS against your applications, they will know if they 
are doing it right or not.

Also, viewers could use Netdata itself to stress your servers. Although the Netdata daemon runs unprivileged, with the minimum 
process priority (scheduling priority `idle` - lower than nice 19) and adjusts its OutOfMemory (OOM) score to 1000 (so that it 
will be first to be killed by the kernel if the system starves for memory), some pressure can be applied on your systems if 
someone attempts a DDoS against Netdata.

Instead of dictating how to secure your infrastructure, we give you many options to establish security best practices
that align with your goals and your organization's standards.

- [Disable the local dashboard](#disable-the-local-dashboard): **Simplest and recommended method** for those who have
  added nodes to Netdata Cloud and view dashboards and metrics there.

- [Expose Netdata only in a private LAN](#expose-netdata-only-in-a-private-lan). Simplest and recommended method for those who do not use Netdata Cloud.

- [Fine-grained access control](#fine-grained-access-control): Allow local dashboard access from
  only certain IP addresses, such as a trusted static IP or connections from behind a management LAN. Full support for Netdata Cloud.

- [Use a reverse proxy (authenticating web server in proxy mode)](#use-an-authenticating-web-server-in-proxy-mode): Password-protect 
  a local dashboard and enable TLS to secure it. Full support for Netdata Cloud.

- [Use Netdata parents as Web Application Firewalls](#use-netdata-parents-as-web-application-firewalls)

- [Other methods](#other-methods) list some less common methods of protecting Netdata.

## Disable the local dashboard

This is the _recommended method for those who have connected their nodes to Netdata Cloud_ and prefer viewing real-time
metrics using the War Room Overview, Nodes tab, and Cloud dashboards.

You can disable the local dashboard (and API) but retain the encrypted Agent-Cloud link 
([ACLK](https://github.com/netdata/netdata/blob/master/src/aclk/README.md)) that
allows you to stream metrics on demand from your nodes via the Netdata Cloud interface. This change mitigates all
concerns about revealing metrics and system design to the internet at large, while keeping all the functionality you
need to view metrics and troubleshoot issues with Netdata Cloud.

Open `netdata.conf` with `./edit-config netdata.conf`. Scroll down to the `[web]` section, and find the `mode =
static-threaded` setting, and change it to `none`.

```conf
[web]
    mode = none
```

Save and close the editor, then [restart your Agent](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) 
using `sudo systemctl
restart netdata`. If you try to visit the local dashboard to `http://NODE:19999` again, the connection will fail because
that node no longer serves its local dashboard.

> See the [configuration basics doc](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) for details on how to find 
`netdata.conf` and use
> `edit-config`.

## Expose Netdata only in a private LAN

If your organisation has a private administration and management LAN, you can bind Netdata on this network interface on all your servers. 
This is done in `Netdata.conf` with these settings:

```
[web]
	bind to = 10.1.1.1:19999 localhost:19999
```

You can bind Netdata to multiple IPs and ports. If you use hostnames, Netdata will resolve them and use all the IPs 
(in the above example `localhost` usually resolves to both `127.0.0.1` and `::1`).

**This is the best and the suggested way to protect Netdata**. Your systems **should** have a private administration and management 
LAN, so that all management tasks are performed without any possibility of them being exposed on the internet.

For cloud based installations, if your cloud provider does not provide such a private LAN (or if you use multiple providers), 
you can create a virtual management and administration LAN with tools like `tincd` or `gvpe`. These tools create a mesh VPN 
allowing all servers to communicate securely and privately. Your administration stations join this mesh VPN to get access to 
management and administration tasks on all your cloud servers.

For `gvpe` we have developed a [simple provisioning tool](https://github.com/netdata/netdata-demo-site/tree/master/gvpe) you 
may find handy (it includes statically compiled `gvpe` binaries for Linux and FreeBSD, and also a script to compile `gvpe` 
on your macOS system). We use this to create a management and administration LAN for all Netdata demo sites (spread all over 
the internet using multiple hosting providers).

## Fine-grained access control

If you want to keep using the local dashboard, but don't want it exposed to the internet, you can restrict access with
[access lists](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#access-lists). This method also fully 
retains the ability to stream metrics
on-demand through Netdata Cloud.

The `allow connections from` setting helps you allow only certain IP addresses or FQDN/hostnames, such as a trusted
static IP, only `localhost`, or connections from behind a management LAN. 

By default, this setting is `localhost *`. This setting allows connections from `localhost` in addition to _all_
connections, using the `*` wildcard. You can change this setting using Netdata's [simple
patterns](https://github.com/netdata/netdata/blob/master/src/libnetdata/simple_pattern/README.md).

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

See the [web server](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#access-lists) docs for additional details
about access lists. You can take
access lists one step further by [enabling SSL](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#enabling-tls-support) to encrypt data from local
dashboard in transit. The connection to Netdata Cloud is always secured with TLS.

## Use an authenticating web server in proxy mode

Use one web server to provide authentication in front of **all your Netdata servers**. So, you will be accessing all your Netdata with 
URLs like `http://{HOST}/netdata/{NETDATA_HOSTNAME}/` and authentication will be shared among all of them (you will sign-in once for all your servers). 
Instructions are provided on how to set the proxy configuration to have Netdata run behind 
[nginx](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md), 
[HAproxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md), 
[Apache](https://github.com/netdata/netdata/blob/master/docs/Running-behind-apache.md), 
[lighthttpd](https://github.com/netdata/netdata/blob/master/docs/Running-behind-lighttpd.md), 
[caddy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-caddy.md), and
[H2O](https://github.com/netdata/netdata/blob/master/docs/Running-behind-h2o.md).

## Use Netdata parents as Web Application Firewalls

The Netdata Agents you install on your production systems do not need direct access to the Internet. Even when you use 
Netdata Cloud, you can appoint one or more Netdata Parents to act as border gateways or application firewalls, isolating 
your production systems from the rest of the world. Netdata 
Parents receive metric data from Netdata Agents or other Netdata Parents on one side, and serve most queries using their own 
copy of the data to satisfy dashboard requests on the other side.

For more information see [Streaming and replication](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md).

## Other methods

Of course, there are many more methods you could use to protect Netdata:

-   Bind Netdata to localhost and use `ssh -L 19998:127.0.0.1:19999 remote.netdata.ip` to forward connections of local port 19998 to remote port 19999. 
This way you can ssh to a Netdata server and then use `http://127.0.0.1:19998/` on your computer to access the remote Netdata dashboard.

-   If you are always under a static IP, you can use the script given above to allow direct access to your Netdata servers without authentication, 
from all your static IPs.

-   Install all your Netdata in **headless data collector** mode, forwarding all metrics in real-time to a parent
    Netdata server, which will be protected with authentication using an nginx server running locally at the parent
    Netdata server. This requires more resources (you will need a bigger parent Netdata server), but does not require
    any firewall changes, since all the child Netdata servers will not be listening for incoming connections.
