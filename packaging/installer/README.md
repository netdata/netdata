import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'
import { InstallRegexLink, InstallBoxRegexLink } from '@site/src/components/InstallRegexLink/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Netdata Agent Installation

Netdata is very flexible and can be used to monitor all kinds of infrastructure. Read more about possible [Deployment guides](https://github.com/netdata/netdata/blob/master/docs/deployment-guides/README.md) to understand what better suites your needs.

## Install through Netdata Cloud

Netdata is a free and open-source (FOSS) monitoring agent that collects thousands of hardware and software metrics from any physical or virtual system (we call them _nodes_). These metrics are organized in an easy-to-use and -navigate interface.

Netdata runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.
It runs on Linux distributions (Ubuntu, Debian, CentOS, and more), container/microservice platforms (Kubernetes clusters, Docker), and many other operating systems (FreeBSD, macOS), with no `sudo` required.

To install Netdata in minutes on your platform:

1. Sign up to <https://app.netdata.cloud/>
2. You will be presented with an empty space, and a prompt to "Connect Nodes" with the install command for each platform
3. Select the platform you want to install Netdata to, copy and paste the script into your node's terminal, and run it

Upon installation completing successfully, you should be able to see the node live in your Netdata Space and live charts in the Overview tab. [Take a look at our Dashboards and Charts](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md) section to read more about Netdata's features.

## Maintaining a Netdata Agent installation

For actions like starting, stopping, restarting, updating and uninstalling the Netdata Agent take a look at your specific installation platform in the current section of our Documentation.

## Configuration

If you are looking to configure your Netdata Agent installation, refer to the [respective section in our Documentation](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md).

## Data collection

If Netdata didn't autodetect all the hardware, containers, services, or applications running on your node, you should learn more about [how data collectors work](https://github.com/netdata/netdata/blob/master/src/collectors/README.md). If there's a [supported collector](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md) for metrics you need, [configure the collector](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md) or read about its requirements to configure your endpoint to publish metrics in the correct format and endpoint.

## Alerts & notifications

Netdata comes with hundreds of pre-configured alerts, designed by our monitoring gurus in parallel with our open-source community, but you may want to [edit alerts](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) or [enable notifications](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/README.md) to customize your Netdata experience.

## Make your deployment production ready

Go through our [deployment guides](https://github.com/netdata/netdata/blob/master/docs/deployment-guides/README.md), for suggested configuration changes for production deployments.

## Advanced installation options and troubleshooting

### Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to update your Netdata agent manually, you can disable automatic updates by using the `--no-updates`
option when you install or update Netdata using the [automatic one-line installation
script](#automatic-one-line-installation-script).

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --no-updates
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

#### Installs on hosts without IPv4 connectivity

Our regular installation process requires access to a number of GitHub services that do not have IPv6 connectivity. As
such, using the kickstart install script on such hosts generally does not work, and will typically fail with an
error from cURL or wget about connection timeouts. You can check if your system is affected by this by attempting
to connect to (or ping) `https://api.github.com/`. Failing to connect indicates that you are affected by this issue.

There are three potential workarounds for this:

1. You can configure your system with a proper IPv6 transition mechanism, such as NAT64. GitHub’s anachronisms
   affect many projects other than just Netdata, and there are unfortunately a number of other services out there
   that do not provide IPv6 connectivity, so taking this route is likely to save you time in the future as well.
2. If you are using a system that we publish native packages for (see our [platform support
   policy](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/versions-and-platforms.md) for more details),
   you can manually set up our native package repositories as outlined in our [native package install
   documentation](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md). Our official
   package repositories do provide service over IPv6, so they work without issue on hosts without IPv4 connectivity.
3. If neither of the above options work for you, you can still install using our [offline installation
   instructions](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/offline.md), though
   do note that the offline install source must be prepared from a system with IPv4 connectivity.

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

If you see an error similar to `Access to file is not permitted: /usr/share/netdata/web/index.html` when you try to
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
# chown -R netdata:netdata /usr/share/netdata/web
```

#### Multiple versions of OpenSSL

We've received reports from the community about issues with running the `kickstart.sh` script on systems that have both
a distribution-installed version of OpenSSL and a manually-installed local version. The Agent's installer cannot handle
both.

#### Clang compiler on Linux

Our current build process has some issues when using certain configurations of the `clang` C compiler on Linux. See [the
section on `nonrepresentable section on output`
errors](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#nonrepresentable-section-on-output-errors) for a workaround.
