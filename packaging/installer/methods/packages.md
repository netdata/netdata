<!--
title: "Install Netdata with .deb/.rpm packages"
description: "Install the Netdata Agent with Linux packages that support Ubuntu, Debian, Fedora, RHEL, CentOS, openSUSE, and more."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/packages.md
-->

# Install Netdata with .deb/.rpm packages

Netdata provides our own flavour of binary packages for the most common operating systems that use with `.deb` and
`.rpm` packaging formats.

We provide two separate repositories, one for our stable releases and one for our nightly releases. Visit the repository
pages and follow the quick set-up instructions to get started.

1.  Stable releases: Our stable production releases are hosted in the 
    [netdata/netdata](https://packagecloud.io/netdata/netdata) repository on packagecloud
2.  Nightly releases: Our latest releases are hosted in the
    [netdata/netdata-edge](https://packagecloud.io/netdata/netdata-edge) repository on packagecloud

## Using caching proxies with packagecloud repositories

packagecloud only provides HTTPS access to repositories they host, which means in turn that Netdata's package
repositories are only accessible via HTTPS. This is known to cause issues with some setups that use a caching proxy for
package downloads.

If you are using such a setup, there are a couple of ways to work around this:

-   Configure your proxy to automatically pass through HTTPS connections without caching them. This is the simplest
    solution, but means that downloads of Netdata packages will not be cached.
-   Mirror the repository locally on your proxy system, and use that mirror when installing on other systems. This
    requires more setup and more disk space on the caching host, but it lets you cache the packages locally.
-   Some specific caching proxies may have alternative configuration options to deal with these issues. Find
    such options in their documentation.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackages%2Finstaller%2Fmethods%2Fpackages&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
