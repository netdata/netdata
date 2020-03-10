# Installation guide

Netdata is a monitoring agent designed to run on all your systems: physical and virtual servers, containers, even
IoT/edge devices. Netdata runs on Linux, FreeBSD, macOS, Kubernetes, Docker, and all their derivatives.

The best way to install Netdata is with our [**automatic one-line installation
script**](#automatic-one-line-installation-script), which works with all Linux distributions, or our [**.deb/rpm
packages**](methods/packages.md), which seamlessly install with your distribution's package manager.

If you want to install Netdata with Docker, on a Kubernetes cluster, or a different operating system, see [Have a
different operating system, or want to try another
method?](#have-a-different-operating-system-or-want-to-try-another-method)

Some third parties, such as the packaging teams at various Linux distributions, distribute old, broken, or altered
packages. We recommend you install Netdata using one of the methods listed below to guarantee you get the latest
checksum-verified packages.

Starting with v1.12, Netdata collects anonymous usage information by default and sends it to Google Analytics. Read
about the information collected, and learn how to-opt, on our [anonymous statistics](../../docs/anonymous-statistics.md)
page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

## Automatic one-line installation script

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is fully automatic on all Linux distributions, including Ubuntu, Debian, Fedora, CentOS, and others.

To install Netdata from source and get _automatic nightly updates_, run the following as your normal user:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

To see more information about this installation script, including how to disable automatic updates, get nightly vs.
stable releases, or disable anonymous statistics, see the [`kickstart.sh` method page](methods/kickstart.md). 

Scroll down for details about [automatic updates](#automatic-updates) or [nightly vs. stable
releases](#nightly-vs-stable-releases).

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs. 

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../docs/getting-started.md) for a quick overview.

## Have a different operating system, or want to try another method?

Netdata works on many different operating systems, each with a few possible installation methods. To see the full list
of approved methods for each operating system/version we support, see our [distribution matrix](../DISTRIBUTIONS.md).

Below, you can find a few additional installation methods, followed by separate instructions for a variety of unique
operating systems.

### Alternative methods

<div class="installer-grid" markdown=1>

[![Install with .deb or .rpm
packages](https://user-images.githubusercontent.com/1153921/76029431-aebd6b00-5ef1-11ea-92b4-06704dabb93e.png) Install
with .deb or .rpm packages](methods/packages.md)

[![Install with a pre-built static binary for 64-bit
systems](https://user-images.githubusercontent.com/1153921/73030303-94727680-3df6-11ea-963e-6f2cb0ce762c.png) Install
with a pre-built static binary for 64-bit systems](methods/kickstart-64.md)

[![Install Netdata on
Docker](https://user-images.githubusercontent.com/1153921/76029355-85044400-5ef1-11ea-96f4-79edc58f9b5c.png) Install
Netdata on Docker](../docker/README.md)

[![Install Netdata on
Kubernetes](https://user-images.githubusercontent.com/1153921/76029478-cc8ad000-5ef1-11ea-8981-dd04744b00da.png) Install
Netdata on Kubernetes with a Helm
chart](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)

[![Install Netdata on cloud providers
(GCP/AWS/Azure)](https://user-images.githubusercontent.com/1153921/76029431-aebd6b00-5ef1-11ea-92b4-06704dabb93e.png)
Install Netdata on cloud providers (GCP/AWS/Azure)](methods/cloud-providers.md)

[![Install Netdata on
macOS](https://user-images.githubusercontent.com/1153921/76029616-1673b600-5ef2-11ea-888a-4a1375a42246.png) Install
Netdata on macOS](methods/macos.md)

[![Install Netdata on
FreeBSD](https://user-images.githubusercontent.com/1153921/76029787-5fc40580-5ef2-11ea-9461-23e9049aa8f8.png) Install
Netdata on FreeBSD](methods/freebsd.md)

[![Install manually from
source](https://user-images.githubusercontent.com/1153921/73032280-f1246000-3dfb-11ea-870d-7fbddd9a6f76.png) Install
manually from source](methods/manual.md)

[![Install on offline/air-gapped
systems](https://user-images.githubusercontent.com/1153921/73032239-c89c6600-3dfb-11ea-8224-c8a9f7a50c53.png) Install on
offline/air-gapped systems](methods/offline.md)

[![Installation on
PFSense](https://user-images.githubusercontent.com/1153921/76030071-cb0dd780-5ef2-11ea-87cd-607d943dc521.png)
Installation on PFSense](methods/pfsense.md)

[![Install Netdata on
Synology](https://user-images.githubusercontent.com/1153921/76029789-5fc40580-5ef2-11ea-9d35-c022f682da77.png) Install
Netdata on Synology](methods/synology.md)

[![Manual installation on
FreeNAS](https://user-images.githubusercontent.com/1153921/76030537-1c1dcb80-5ef3-11ea-9cf9-f130e7d41712.png) Manual
installation on FreeNAS](methods/freenas.md)

[![Manual installation on
Alpine](https://user-images.githubusercontent.com/1153921/76029682-37d4a200-5ef2-11ea-9a2c-a8ffeb1d13c3.png) Manual
installation on Alpine](methods/alpine.md)

</div>

## Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to update your Netdata agent manually, you can disable automatic updates by using the `--no-updates`
option when you install or update Netdata using the [automatic one-line installation
script](#automatic-one-line-installation-script).

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update Netdata](UPDATE.md).

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
