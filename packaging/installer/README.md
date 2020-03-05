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

<div class="installer-grid">
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/73030393-c5eb4200-3df6-11ea-9942-436caa3ed100.png" alt="Install with .deb or .rpm packages" />
      <h3>Packages</h3>
    </div>
    <ul>
      <li><a href="methods/packages/">Install with <code>.deb</code> or <code>.rpm</code> packages</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/73030303-94727680-3df6-11ea-963e-6f2cb0ce762c.png" alt="Install with a pre-built static binary for 64-bit systems" />
      <h3>Static binary</h3>
    </div>
    <ul>
      <li><a href="methods/kickstart-64/">Install with a pre-built static binary for 64-bit systems</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71905478-e36ea980-3170-11ea-94f7-950328ad1bdf.png" alt="Install Netdata on Docker" />
      <h3>Docker</h3>
    </div>
    <ul>
      <li><a href="../docker/#run-netdata-with-the-docker-command">Using the <code>docker</code> command</a></li>
      <li><a href="../docker/#run-netdata-with-the-docker-command">Using a <code>docker-compose.yml</code> file</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71960868-c1236d00-31fe-11ea-859e-902d36233e38.png" alt="Install Netdata on Kubernetes" />
      <h3>Kubernetes</h3>
    </div>
    <ul>
      <li><a href="https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments">Using a Helm chart</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/73030393-c5eb4200-3df6-11ea-9942-436caa3ed100.png" alt="Install Netdata on cloud providers (GCP/AWS/Azure)" />
      <h3>Cloud providers (GCP/AWS/Azure)</h3>
    </div>
    <ul>
      <li><a href="methods/cloud-providers/#recommended-installation-method-for-cloud-providers">Recommended installation methods for cloud providers</a></li>
      <li><a href="methods/cloud-providers/#post-installation-configuration">Post-installation configuration</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71961672-8cb0b080-3200-11ea-84f8-9139c7434110.png" alt="Install Netdata on macOS" />
      <h3>macOS</h3>
    </div>
    <ul>
      <li><a href="methods/macos/#with-homebrew">Homebrew</a></li>
      <li><a href="methods/macos/#from-source">Manual installation from source</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71961245-a3a2d300-31ff-11ea-89bf-b90e7242d9a5.png" alt="Install Netdata on FreeBSD" />
      <h3>FreeBSD</h3>
    </div>
    <ul>
      <li><a href="methods/freebsd/">Installation on FreeBSD</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/73032280-f1246000-3dfb-11ea-870d-7fbddd9a6f76.png" alt="Install manually from source" />
      <h3>Manual</h3>
    </div>
    <ul>
      <li><a href="methods/manual/">Install manually from source</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/73032239-c89c6600-3dfb-11ea-8224-c8a9f7a50c53.png" alt="Install on offline/air-gapped systems" />
      <h3>Offline</h3>
    </div>
    <ul>
      <li><a href="methods/offline/">Install on offline/air-gapped systems</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71961918-13fe2400-3201-11ea-9a91-fe6f5b27df0c.png" alt="Install Netdata on PFSense" />
      <h3>PFSense</h3>
    </div>
    <ul>
      <li><a href="methods/pfsense/">Installation on PFSense</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/71962148-853dd700-3201-11ea-9a09-16fdb39e9ee4.png" alt="Install Netdata on Synology" />
      <h3>Synology</h3>
    </div>
    <ul>
      <li><a href="methods/synology/">Installation on Synology</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/72070923-543dcf00-32f3-11ea-8053-d61bc96529b5.png" alt="Install Netdata on Alpine FreeNAS" />
      <h3>FreeNAS</h3>
    </div>
    <ul>
      <li><a href="methods/freenas/">Manual installation on FreeNAS</a></li>
    </ul>
  </div>
  <div class="grid-item">
    <div class="item-title">
      <img src="https://user-images.githubusercontent.com/1153921/72070921-53a53880-32f3-11ea-80f1-7d00cd8a7906.png" alt="Install Netdata on Alpine Linux" />
      <h3>Alpine</h3>
    </div>
    <ul>
      <li><a href="methods/alpine/">Manual installation on Alpine</a></li>
    </ul>
  </div>
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
