<!--
---
title: "Install Netdata on cloud providers"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/cloud-providers.md
---
-->

# Install Netdata on cloud providers

Netdata is fully compatible with popular cloud providers like Google Cloud Platform (GCP), Amazon Web Services (AWS),
Azure, and others. You can install Netdata on cloud instances to monitor the apps/services running there, or use
multiple instances in a [master/slave streaming](../../../streaming/README.md) configuration.

In some cases, using Netdata on these cloud providers requires unique installation or configuration steps. This page
aims to document some of those steps for popular cloud providers.

> This document is a work-in-progress! If you find new issues specific to a cloud provider, or would like to help
> clarify the correct workaround, please [create an
> issue](https://github.com/netdata/netdata/issues/new?labels=feature+request%2C+needs+triage&template=feature_request.md)
> with your process and instructions on using the provider's interface to complete the workaround.

-   [Recommended installation methods for cloud providers](#recommended-installation-methods-for-cloud-providers)
-   [Post-installation configuration](#post-installation-configuration)
    -   [Add a firewall rule to access Netdata's dashboard](#add-a-firewall-rule-to-access-netdatas-dashboard)

## Recommended installation methods for cloud providers

The best installation method depends on the instance's operating system, distribution, and version. For Linux instances,
we recommend either the [`kickstart.sh` automatic installation script](kickstart.md) or [.deb/.rpm
packages](packages.md).

To see the full list of approved methods for each operating system/version we support, see our [distribution
matrix](../../DISTRIBUTIONS.md). That table will guide you to the various supported methods for your cloud instance.

If you have issues with Netdata after installation, look to the sections below to find the issue you're experiencing,
followed by the solution for your provider.

## Post-installation configuration

Some cloud providers require you take additional steps to properly configure your instance or its networking to access
all of Netdata's features.

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

-   [Apache](../../../docs/Running-behind-apache.md)
-   [Nginx](../../../docs/Running-behind-nginx.md)
-   [Caddy](../../../docs/Running-behind-caddy.md)
-   [HAProxy](../../../docs/Running-behind-haproxy.md)
-   [lighttpd](../../../docs/Running-behind-lighttpd.md)

The next few sections outline how to add firewall rules to GCP, AWS, and Azure instances.

#### Google Cloud Platform (GCP)

To add a firewall rule, go to the [Firewall rules page](https://console.cloud.google.com/networking/firewalls/list) and
click **Create firewall rule**.

The following configuration has previously worked for Netdata running on GCP instances
([see #7786](https://github.com/netdata/netdata/issues/7786)):

```conf
Name: <name>
Type: Ingress
Targets: <name-tag>
Filters: 0.0.0.0/0
Protocols/ports: 19999
Action: allow
Priority: 1000
```

Read GCP's [firewall documentation](https://cloud.google.com/vpc/docs/using-firewalls) for specific instructions on how
to create a new firewall rule.

#### Amazon Web Services (AWS) / EC2

Sign in to the [AWS console](https://console.aws.amazon.com/) and navigate to the EC2 dashboard. Click on the **Security
Groups** link in the naviagtion, beneath the **Network & Security** heading. Find the Security Group your instance
belongs to, and either right-click on it or click the **Actions** button above to see a dropdown menu with **Edit
inbound rules**.

Add a new rule with the following options:

```conf
Type: Custom TCP
Protocol: TCP
Port Range: 19999
Source: Anywhere
Description: Netdata
```

You can also choose **My IP** as the source if you prefer.

Click **Save** to apply your new inbound firewall rule.

#### Azure

Sign in to the [Azure portal](https://portal.azure.com) and open the virtual machine running Netdata. Click on the
**Networking** link beneath the **Settings** header, then click on the **Add inbound security rule** button.

Add a new rule with the following options:

```conf
Source: Any
Source port ranges: 19999
Destination: Any
Destination port randes: 19999
Protocol: TCP
Action: Allow
Priority: 310
Name: Netdata
```

Click **Add** to apply your new inbound security rule.
