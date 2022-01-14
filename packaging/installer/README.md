<!--
title: "Installation guide"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/README.md
-->

import { Install, InstallBox } from '../../../src/components/Install/'

# Installation guide

Netdata is a monitoring agent designed to run on all your systems: physical and virtual servers, containers, even
IoT/edge devices. Netdata runs on Linux, FreeBSD, macOS, Kubernetes, Docker, and all their derivatives.

The best way to install Netdata is with our [**automatic one-line installation
script**](#automatic-one-line-installation-script), which works with all Linux distributions and most macOS environments.

If you want to install Netdata with Docker, on a Kubernetes cluster, or a different operating system, see [Have a
different operating system, or want to try another
method?](#have-a-different-operating-system-or-want-to-try-another-method)

Some third parties, such as the packaging teams at various Linux distributions, distribute old, broken, or altered
packages. We recommend you install Netdata using one of the methods listed below to guarantee you get the latest
checksum-verified packages.

Netdata collects anonymous usage information by default and sends it to our self hosted [PostHog](https://github.com/PostHog/posthog) installation. PostHog is an open source product analytics platform, you can read
about the information collected, and learn how to-opt, on our [anonymous statistics](/docs/anonymous-statistics.md)
page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

## Automatic one-line installation script

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is fully automatic on all Linux distributions, including Ubuntu, Debian, Fedora, CentOS, and others, as well as on mac OS environments.

To install Netdata, including all dependencies required to connect to Netdata Cloud, and get _automatic nightly
updates_, run the following as your normal user:

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh
```

This script will preferentially use native DEB/RPM packages if we provide them for your platform.

To see more information about this installation script, including how to disable automatic updates, get nightly vs.
stable releases, or disable anonymous statistics, see the [`kickstart.sh` method
page](/packaging/installer/methods/kickstart.md).

Scroll down for details about [automatic updates](#automatic-updates) or [nightly vs. stable
releases](#nightly-vs-stable-releases).

### Post-installation

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

## Have a different operating system, or want to try another method?

Netdata works on many different platforms. To see all supported platforms, check out our [platform support
policy](/packaging/PLATFORM_SUPPORT.md).

Below, you can find a few additional installation methods, followed by separate instructions for a variety of unique
operating systems.

### Alternative methods

<Install>
  <InstallBox
    to="/docs/agent/packaging/installer/methods/kickstart"
    os="General Linux with one-line installer (recommended)"
    svg="linux" />
  <InstallBox
    to="/docs/agent/packaging/docker"
    os="Run with Docker"
    svg="docker" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/kubernetes"
    os="Deploy on Kubernetes"
    svg="kubernetes" />
   <InstallBox
    to="/docs/agent/packaging/installer/methods/macos"
    os="Install on macOS"
    svg="macos" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/manual"
    os="Linux from Git"
    svg="linux" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/source"
    os="Linux from source"
    svg="linux" />
  <InstallBox
    to="/docs/agent/packaging/installer/methods/offline" 
    os="Linux for offline nodes"
    svg="linux" />
</Install>

## Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to update your Netdata agent manually, you can disable automatic updates by using the `--no-updates`
option when you install or update Netdata using the [automatic one-line installation
script](#automatic-one-line-installation-script).

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update
Netdata](/packaging/installer/UPDATE.md).

### Network usage of Netdata’s automatic updater

The auto-update functionality set up by the installation scripts requires working internet access to function
correctly. In particular, it currently requires access to GitHub (to check if a newer version of the updater script
is available or not, as well as potentially fetching build-time dependencies that are bundled as part of the install),
and Google Cloud Storage (to check for newer versions of Netdata and download the sources if there is a newer version).

Note that the auto-update functionality will check for updates to itself independently of updates to Netdata,
and will try to use the latest version of the updater script whenever possible. This is intended to reduce the
amount of effort required by users to get updates working again in the event of a bug in the updater code.

## Nightly vs. stable releases

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

-   Get the latest features and bug fixes as soon as they're available
-   Receive security-related fixes immediately
-   Use stable, fully-tested code that's always improving
-   Leverage the same Netdata experience our community is using

**Pros of using stable releases:**

-   Protect yourself from the rare instance when major bugs slip through our testing and negatively affect a Netdata
    installation
-   Retain more control over the Netdata version you use

## Troubleshooting and known issues

We are tracking a few issues related to installation and packaging.

### Older distributions (Ubuntu 14.04, Debian 8, CentOS 6) and OpenSSL

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS
6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old
versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which
helps securely encrypt SSL connections.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit
with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to
man-in-the-middle attacks.

### CentOS 6 and CentOS 8

To install the Agent on certain CentOS and RHEL systems, you must enable non-default repositories, such as EPEL or
PowerTools, to gather hard dependencies. See the [CentOS 6](/packaging/installer/methods/manual.md#centos--rhel-6x) and
[CentOS 8](/packaging/installer/methods/manual.md#centos--rhel-8x) sections for more information.

### Access to file is not permitted

If you see an error similar to `Access to file is not permitted: /usr/share/netdata/web//index.html` when you try to
visit the Agent dashboard at `http://NODE:19999`, you need to update Netdata's permissions to match those of your
system.

Run `ls -la /usr/share/netdata/web/index.html` to find the file's permissions. You may need to change this path based on
the error you're seeing in your browser. In the below example, the file is owned by the user `netdata` and the group
`netdata`.

```bash
ls -la /usr/share/netdata/web/index.html
-rw-r--r--. 1 netdata netdata 89377 May  5 06:30 /usr/share/netdata/web/index.html
```

Open your `netdata.conf` file and find the `[web]` section, plus the `web files owner`/`web files group` settings. Edit
the lines to match the output from `ls -la` above and uncomment them if necessary.

```conf
[web]
    web files owner = netdata
    web files group = netdata
```

Save the file, restart Netdata using `sudo systemctl restart netdata`, or the [appropriate
method](/docs/configure/start-stop-restart.md) for your system, and try accessing the dashboard again.

### Multiple versions of OpenSSL

We've received reports from the community about issues with running the `kickstart.sh` script on systems that have both
a distribution-installed version of OpenSSL and a manually-installed local version. The Agent's installer cannot handle
both.

### Clang compiler on Linux

Our current build process has some issues when using certain configurations of the `clang` C compiler on Linux. See [the
section on `nonrepresentable section on output`
errors](/packaging/installer/methods/manual.md#nonrepresentable-section-on-output-errors) for a workaround.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
