import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'
import { InstallRegexLink, InstallBoxRegexLink } from '@site/src/components/InstallRegexLink/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata

This document will guide you through installing the open-source Netdata monitoring Agent on Linux, Docker, Kubernetes, and many others, often with one command.

## Get started

Netdata is a free and open-source (FOSS) monitoring agent that collects thousands of hardware and software metrics from
any physical or virtual system (we call them _nodes_). These metrics are organized in an easy-to-use and -navigate interface.

Together with [Netdata Cloud](https://github.com/netdata/netdata/blob/master/docs/quickstart/infrastructure.md), you can monitor your entire infrastructure in
real time and troubleshoot problems that threaten the health of your nodes.

Netdata runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices. It
runs on Linux distributions (Ubuntu, Debian, CentOS, and more), container/microservice platforms (Kubernetes clusters,
Docker), and many other operating systems (FreeBSD, macOS), with no `sudo` required.

To install Netdata in minutes on your platform:

1. Sign up to <https://app.netdata.cloud/>
2. You will be presented with an empty space, and a prompt to "Connect Nodes" with the install command for each platform
3. Select the platform you want to install Netdata to, copy and paste the script into your node's terminal, and run it

Upon installation completing successfully, you should be able to see the node live in your Netdata Space and live charts
in the Overview tab. [Read more about the cloud features](https://github.com/netdata/netdata/blob/master/docs/quickstart/infrastructure.md).

Where you go from here is based on your use case, immediate needs, and experience with monitoring and troubleshooting,
but we have some hints on what you might want to do next.

### What's next?

Explore our [general advanced installation options and troubleshooting](#advanced-installation-options-and-troubleshooting), specific options
for the [single line installer](#install-on-linux-with-one-line-installer), or [other installation methods](#other-installation-methods).

#### Agent user interface

To access the UI provided by the locally installed agent, open a browser and navigate to `http://NODE:19999`, replacing `NODE` with either `localhost` or
the hostname/IP address of the remote node. You can also read more about
[the agent dashboard](https://github.com/netdata/netdata/blob/master/web/gui/README.md).

#### Configuration

Discover the recommended way to [configure Netdata's settings or behavior](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) using our built-in
`edit-config` script, then apply that knowledge to mission-critical tweaks, such as [changing how long Netdata stores
metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md).

#### Data collection

If Netdata didn't autodetect all the hardware, containers, services, or applications running on your node, you should
learn more about [how data collectors work](https://github.com/netdata/netdata/blob/master/collectors/README.md). If there's a [supported
collector](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md) for metrics you need, [configure the collector](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md)
or read about its requirements to configure your endpoint to publish metrics in the correct format and endpoint.

#### Alarms & notifications

Netdata comes with hundreds of preconfigured alarms, designed by our monitoring gurus in parallel with our open-source
community, but you may want to [edit alarms](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) or
[enable notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) to customize your Netdata experience.

#### Make your deployment production ready

Go through our [deployment strategies](https://github.com/netdata/netdata/edit/master/docs/category-overview-pages/deployment-strategies.md),
for suggested configuration changes for production deployments.

## Install on Linux with one-line installer

The **recommended** way to install Netdata on a Linux node (physical, virtual, container, IoT) is our one-line
[kickstart script](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md).
This script automatically installs dependencies and builds Netdata from its source code.

To install, copy the script, paste it into your node's terminal, and hit `Enter` to begin the installation process.

 <Tabs>
  <TabItem value="wget" label=<code>wget</code>>

  <OneLineInstallWget/>

  </TabItem>
  <TabItem value="curl" label=<code>curl</code>>

  <OneLineInstallCurl/>

  </TabItem>
</Tabs>

> ### Note
>
> If you plan to also claim the node to Netdata Cloud, make sure to replace `YOUR_CLAIM_TOKEN` with the claim token of your space, and `YOUR_ROOM_ID` with the ID of the room you are claiming to.
> You can leave the room id blank to have your node claimed to the default "All nodes" room.

Jump down to [what's next](#whats-next) to learn how to view your new dashboard and take your next steps monitoring and
troubleshooting with Netdata.

## Other installation methods

<InstallRegexLink>
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md)"
    os="Run with Docker"
    svg="docker" />
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kubernetes.md)"
    os="Deploy on Kubernetes"
    svg="kubernetes" />
   <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/macos.md)"
    os="Install on macOS"
    svg="macos" />
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md)"
    os="Native DEB/RPM packages"
    svg="linux" />
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md)"
    os="Linux from Git"
    svg="linux" />
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/source.md)"
    os="Linux from source"
    svg="linux" />
  <InstallBoxRegexLink
    to="[](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/offline.md)"
    os="Linux for offline nodes"
    svg="linux" />
</InstallRegexLink>

- [Native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md)
- [Run with Docker](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md)
- [Deploy on Kubernetes](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kubernetes.md)
- [Install on macOS](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/macos.md)
- [Linux from Git](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md)
- [Linux from source](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/source.md)
- [Linux for offline nodes](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/offline.md)

The full list of all installation methods for various systems is available in [Netdata Learn](https://learn.netdata.cloud),
under [Installation](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/installation-overview.md).

## Advanced installation options and troubleshooting

### Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to update your Netdata agent manually, you can disable automatic updates by using the `--no-updates`
option when you install or update Netdata using the [automatic one-line installation
script](#automatic-one-line-installation-script).

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update
Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/UPDATE.md).

#### Network usage of Netdata’s automatic updater

The auto-update functionality set up by the installation scripts requires working internet access to function
correctly. In particular, it currently requires access to GitHub (to check if a newer version of the updater script
is available or not, as well as potentially fetching build-time dependencies that are bundled as part of the install),
and Google Cloud Storage (to check for newer versions of Netdata and download the sources if there is a newer version).

Note that the auto-update functionality will check for updates to itself independently of updates to Netdata,
and will try to use the latest version of the updater script whenever possible. This is intended to reduce the
amount of effort required by users to get updates working again in the event of a bug in the updater code.

### Nightly vs. stable releases

The Netdata team maintains two releases of the Netdata agent: **nightly** and **stable**. By default, Netdata's
installation scripts will give you **automatic, nightly** updates, as that is our recommended configuration.

**Nightly**: We create nightly builds every 24 hours. They contain fully-tested code that fixes bugs or security flaws,
or introduces new features to Netdata. Every nightly release is a candidate for then becoming a stable release—when
we're ready, we simply change the release tags on GitHub. That means nightly releases are stable and proven to function
correctly in the vast majority of Netdata use cases. That's why nightly is the _best choice for most Netdata users_.

**Stable**: We create stable releases whenever we believe the code has reached a major milestone. Most often, stable
releases correlate with the introduction of new, significant features. Stable releases might be a better choice for
those who run Netdata in _mission-critical production systems_, as updates will come more infrequently, and only after
the community helps fix any bugs that might have been introduced in previous releases.

**Pros of using nightly releases:**

- Get the latest features and bug fixes as soon as they're available
- Receive security-related fixes immediately
- Use stable, fully-tested code that's always improving
- Leverage the same Netdata experience our community is using

**Pros of using stable releases:**

- Protect yourself from the rare instance when major bugs slip through our testing and negatively affect a Netdata
    installation
- Retain more control over the Netdata version you use

### Anonymous statistics

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self-hosted PostHog instance within the Netdata infrastructure. Read about the information collected, and learn how to-opt, on our [anonymous statistics](https://github.com/netdata/netdata/blob/master/docs/anonymous-statistics.md) page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

### Troubleshooting and known issues

We are tracking a few issues related to installation and packaging.

#### Older distributions (Ubuntu 14.04, Debian 8, CentOS 6) and OpenSSL

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS
6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old
versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which
helps securely encrypt SSL connections.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit
with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to
man-in-the-middle attacks.

#### CentOS 6 and CentOS 8

To install the Agent on certain CentOS and RHEL systems, you must enable non-default repositories, such as EPEL or
PowerTools, to gather hard dependencies. See the [CentOS 6](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#centos--rhel-6x) and
[CentOS 8](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#centos--rhel-8x) sections for more information.

#### Access to file is not permitted

If you see an error similar to `Access to file is not permitted: /usr/share/netdata/web//index.html` when you try to
visit the Agent dashboard at `http://NODE:19999`, you need to update Netdata's permissions to match those of your
system.

Run `ls -la /usr/share/netdata/web/index.html` to find the file's permissions. You may need to change this path based on
the error you're seeing in your browser. In the below example, the file is owned by the user `root` and the group
`root`.

```bash
ls -la /usr/share/netdata/web/index.html
-rw-r--r--. 1 root root 89377 May  5 06:30 /usr/share/netdata/web/index.html
```

These files need to have the same user and group used to install your netdata. Suppose you installed netdata with user
`netdata` and group `netdata`, in this scenario you will need to run the following command to fix the error:

```bash
# chown -R netdata.netdata /usr/share/netdata/web
```

#### Multiple versions of OpenSSL

We've received reports from the community about issues with running the `kickstart.sh` script on systems that have both
a distribution-installed version of OpenSSL and a manually-installed local version. The Agent's installer cannot handle
both.

#### Clang compiler on Linux

Our current build process has some issues when using certain configurations of the `clang` C compiler on Linux. See [the
section on `nonrepresentable section on output`
errors](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#nonrepresentable-section-on-output-errors) for a workaround.
