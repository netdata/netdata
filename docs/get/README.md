<!--
title: "Get Netdata"
description: "Time to get Netdata's monitoring and troubleshooting solution. Sign in to Cloud, download the Agent everywhere, and connect it all together."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/get/README.md
-->

# Get Netdata

import { OneLineInstall } from '../src/components/OneLineInstall/'
import { Install, InstallBox } from '../src/components/InstallBox/'

Netdata uses the open-source Netdata Agent and Netdata Cloud web application
[together](/docs/overview/what-is-netdata.md) to help you collect every metric, visualize the health of your nodes, and
troubleshoot complex performance problems. Once you've signed in to Netdata Cloud and installed the Netdata Agent on all
your nodes, you can claim your nodes and see their real-time metrics on a single interface.

## Sign in to Netdata Cloud

If you don't already have a free Netdata Cloud account, go ahead and [create one](https://app.netdata.cloud).

Choose your preferred authentication method and follow the onboarding process to create your Space.

## Install the Netdata Agent

The Netdata Agent runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT
devices. It runs on Linux distributions (**Ubuntu**, **Debian**, **CentOS**, and more), container/microservice platforms
(**Kubernetes** clusters, **Docker**), and many other operating systems (**FreeBSD**, **macOS**), with no `sudo`
required.

> ⚠️ Many distributions ship with third-party packages of Netdata, which we cannot maintain or keep up-to-date. For the
> best experience, use one of the methods described or linked to below.

The **recommended** way to install the Netdata Agent on a Linux system is our one-line [kickstart
script](/packaging/installer/methods/kickstart.md). This script automatically installs dependencies and builds Netdata
from its source code.

<OneLineInstall />

Copy the script, paste it into your node's terminal, and hit `Enter`. 

Open your favorite browser and navigate to `http://localhost:19999` or `http://REMOTE-HOST:19999` to open the dashboard.

<details>
<summary>Watch how the one-line installer works</summary>
<iframe width="820" height="460" src="https://www.youtube.com/embed/tVIp7ycK60A" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>
</details>

### Other operating systems/methods

Want to install Netdata on a Kubernetes cluster, with Docker, or using a different method? Not a Linux user? Choose your
platform to see specific instructions.

<Install>
  <InstallBox
    to="/docs/agent/packaging/installer/methods/kubernetes"
    img="/img/index/methods/kubernetes.svg"
    os="Kubernetes" />
  <InstallBox
    to="/docs/agent/packaging/docker"
    img="/img/index/methods/docker.svg"
    os="Docker" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/cloud-providers"
    img="/img/index/methods/cloud.svg"
    imgDark="/img/index/methods/cloud-dark.svg"
    os="Cloud providers (GCP, AWS, Azure)" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/packages"
    img="/img/index/methods/package.svg"
    imgDark="/img/index/methods/package-dark.svg"
    os="Linux with .deb/.rpm" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/kickstart-64"
    img="/img/index/methods/static.svg"
    imgDark="/img/index/methods/static-dark.svg"
    os="Linux with static 64-bit binary" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/manual" 
    img="/img/index/methods/git.svg"
    imgDark="/img/index/methods/git-dark.svg"
    os="Linux from Git" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/freebsd"
    img="/img/index/methods/freebsd.svg"
    os="FreeBSD" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/macos"
    img="/img/index/methods/macos.svg"
    os="MacOS" />
</Install>

Even more options available in our [packaging documentation](/packaging/installer/README.md#alternative-methods).

## Claim your node on Netdata Cloud

You need to [claim](/claim/README.md) your nodes to see them in Netdata Cloud. Claiming establishes a secure TLS
connection to Netdata Cloud using the [Agent-Cloud link](/aclk/README.md), and proves you have write and administrative
access to that node.

When you view a node in Netdata Cloud, the Agent running on that node streams metrics, metadata, and alarm status to
Netdata Cloud, which in turn streams those metrics to your web browser. Netdata Cloud [does not
store](/docs/store/distributed-data-architecture.md#does-netdata-cloud-store-my-metrics) or log metrics values.

To claim a node, you need to run the claiming script. In Netdata Cloud, click on your Space's name, then **Manage your
Space** in the dropdown. Click **Nodes** in the panel that appears. Copy the script and run it in your node's terminal.
The script looks like the following, with long strings instead of `TOKEN` and `ROOM1,ROOM2`:

```bash
sudo netdata-claim.sh -token=TOKEN -rooms=ROOM1,ROOM2 -url=https://app.netdata.cloud
```

The script returns `Agent was successfully claimed.` after creating a new RSA pair and establishing the link to Netdata
Cloud. If the script returns an error, try our [troubleshooting tips](/claim/README.md#troubleshooting).

> 💡 Our claiming reference guide also contains instructions for claiming [Docker
> containers](/claim/README.md#claim-an-agent-running-in-docker), [Kubernetes cluster parent
> pods](/claim/README.md#claim-an-agent-running-in-docker), via a [proxy](/claim/README.md#claim-through-a-proxy), and
> more.

<details>
<summary>Watch how claiming nodes works</summary>
<iframe width="820" height="460" src="https://www.youtube.com/embed/UAzVvhMab8g" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>
</details>

For more information on the claiming process, why we implemented it, and how it works, see the [claim](/claim/README.md)
and [Agent-Cloud link](/aclk/README.md) reference docs.

## Troubleshooting

If you experience issues with installing the Netdata Agent, see our
[installation](/packaging/installer/README.md#troubleshooting-and-known-issues) reference. Our
[reinstall](/packaging/installer/REINSTALL.md) doc can help clean up your installation and get you back to monitoring.

For Netdata Cloud issues, see the [Netdata Cloud reference docs](https://learn.netdata.cloud/docs/cloud).

## What's next?

At this point, you have set up your free Netdata Cloud account, installed the Netdata Agent on your node(s), and claimed
one or more nodes to your Space. You're ready to start monitoring, visualizing, and troubleshooting with Netdata. We
have two quickstart guides based on the scope of what you need to monitor.

Interested in monitoring a single node? Check out our [single-node monitoring
quickstart](/docs/quickstart/single-node.md).

If you're looking to monitor an entire infrastructure with Netdata, see the [infrastructure monitoring
quickstart](/docs/quickstart/infrastructure.md).

### Related reference documentation

-   [Packaging &amp; installer](/packaging/installer/README.md)
-   [Reinstall Netdata](/packaging/installer/REINSTALL.md)
-   [Update Netdata](/packaging/installer/UPDATE.md)
-   [Agent-Cloud link](/aclk/README.md)
-   [Agent claiming](/claim/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Foverview%2Fnetdata-monitoring-stacka&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
