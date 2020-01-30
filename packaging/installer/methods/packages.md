# Install Netdata with .deb/.rpm packages

![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

This page covers detailed instructions on using `.deb` or `.rpm` packages to install Netdata. 

For certain Linux distributions (see our [distribution matrix](../../DISTRIBUTIONS.md) for supported versions and
architectures), these packages integrate tightly with your system's package manager, making them easier to maintain,
update, and uninstall.

We currently use [packagecloud](https://packagecloud.io/netdata/) to supply repositories and packages. We provide two
separate repositories, one for nightly releases, and another for stable releases.

-   Nightly releases: [netdata/netdata-edge](https://packagecloud.io/netdata/netdata-edge)
-   Stable releases: [netdata/netdata](https://packagecloud.io/netdata/netdata)

Read our notice about [nightly vs. stable releases](../README.md#nightly-vs-stable-releases) to understand the
differences between the two.

## Quickstart

packagecloud offers helper installation scripts to add their repositories to your package manager. Jump to your
distribution and use the commands to install Netdata get _automatic nightly updates_ via your distribution's package
manager. You can also manually add the repositories on [`.deb`
systems](https://packagecloud.io/netdata/netdata-edge/install#manual-deb) or [`.rpm`
systems](https://packagecloud.io/netdata/netdata-edge/install#manual-rpm).

-   [Ubuntu and Debian (`.deb`)](#ubuntu-and-debian-deb)
-   [Fedora (`.rpm`)](#fedora-rpm)

### Ubuntu and Debian (`.deb`)

Use the helper script to add the packagecloud repository, then use `apt` to install Netdata.

```bash
curl -s https://packagecloud.io/install/repositories/netdata/netdata-edge/script.deb.sh | sudo bash
sudo apt-get update
sudo apt-get install netdata
```

Skip ahead to the [What's next?](#whats-next) section to find links to helpful post-installation guides.

### Fedora (`.rpm`)

Use the helper script to add the packagecloud repository, then use `dnf` to install Netdata.

For `.rpm` systems using YUM (Fedora, CentOS, RHEL, OpenSuSE):

```bash
curl -s https://packagecloud.io/install/repositories/netdata/netdata-edge/script.rpm.sh | sudo bash
sudo dnf update
sudo dnf install netdata
```

Skip ahead to the [What's next?](#whats-next) section to find links to helpful post-installation guides.

### Other operating systems

While RHEL, CentOS, and Open SuSE all support `.rpm` packages, full support via these packages is still a
work-in-progress. See our [distribution matrix](../../DISTRIBUTIONS.md) to understand whether you can install Netdata
via `.rpm` packages, and then use your package manager to add the packagecloud repository and install Netdata.

## Using caching proxies with packagecloud repositories

packagecloud only provides HTTPS access to repositories they host, which means in turn that Netdata's package
repositories are only accessible via HTTPS. This is known to cause issues with some setups that use a caching proxy for
package downloads.

If you are using such a setup, there are a couple of ways you can work around this:

-   Configure your proxy to pass through HTTPS connections without caching them automatically. This is the simplest
    solution, but means that downloads of Netdata packages will not be cached.
-   Mirror the repository locally on your proxy system, and use that mirror when installing on other systems. This
    requires more setup and more disk space on the caching host, but it lets you cache the packages locally.
-   Some specific caching proxies may have alternative configuration options to deal with these issues. You can find
    such options in their documentation.

## What's next?

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs.

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../../docs/getting-started.md) for a quick overview.
