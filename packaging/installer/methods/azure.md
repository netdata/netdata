<!--
title: "Install Netdata on Azure"
description: "The Netdata Agent runs on all popular cloud providers, but often requires additional steps and configuration for full functionality."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/azure.md
sidebar_label: "Azure"
learn_status: "Published"
learn_rel_path: "Installation/Install on specific environments"
-->

# Install Netdata on Azure

Netdata is fully compatible with Azure. 
You can install Netdata on cloud instances to monitor the apps/services running there, or use
multiple instances in a [parent-child streaming](https://github.com/netdata/netdata/blob/master/src/streaming/README.md) configuration.

## Recommended installation method

The best installation method depends on the instance's operating system, distribution, and version. For Linux instances,
we recommend the [`kickstart.sh` automatic installation script](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md).

If you have issues with Netdata after installation, look to the sections below to find the issue you're experiencing,
followed by the solution for your provider.

## Post-installation configuration

### Add a firewall rule to access Netdata's dashboard

If you cannot access Netdata's dashboard on your cloud instance via `http://HOST:19999`, and instead get an error page
from your browser that says, "This site can't be reached" (Chrome) or "Unable to connect" (Firefox), you may need to
configure your cloud provider's firewall.

Cloud providers often create network-level firewalls that run separately from the instance itself. Both AWS and Google
Cloud Platform calls them Virtual Private Cloud (VPC) networks. These firewalls can apply even if you've disabled
firewalls on the instance itself. Because you can modify these firewalls only via the cloud provider's web interface,
it's easy to overlook them when trying to configure and access Netdata's dashboard.

You can often confirm a firewall issue by querying the dashboard while connected to the instance via SSH: `curl
http://localhost:19999/api/v1/info`. If you see JSON output, Netdata is running properly. If you try the same `curl`
command from a remote system, and it fails, it's likely that a firewall is blocking your requests.

Another option is to put Netdata behind web server, which will proxy requests through standard HTTP/HTTPS ports
(80/443), which are likely already open on your instance. We have a number of guides available:

-   [Apache](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-apache.md)
-   [Nginx](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md)
-   [Caddy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-caddy.md)
-   [HAProxy](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md)
-   [lighttpd](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-lighttpd.md)

Sign in to the [Azure portal](https://portal.azure.com) and open the virtual machine running Netdata. Click on the
**Networking** link beneath the **Settings** header, then click on the **Add inbound security rule** button.

Add a new rule with the following options:

```conf
Source: Any
Source port ranges: 19999
Destination: Any
Destination port ranges: 19999
Protocol: TCP
Action: Allow
Priority: 310
Name: Netdata
```

Click **Add** to apply your new inbound security rule.


