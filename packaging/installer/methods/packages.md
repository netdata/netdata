# Install Netdata with .deb/.rpm packages

![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

We provide our own flavour of binary packages for the most common operating systems that comply with .RPM and .DEB
packaging formats.

We have currently released packages following the .RPM format with version
[1.16.0](https://github.com/netdata/netdata/releases/tag/v1.16.0). We have planned to release packages following the
.DEB format with version [1.17.0](https://github.com/netdata/netdata/releases/tag/v1.17.0). Early adopters may
experiment with our .DEB formatted packages using our nightly releases. Our current packaging infrastructure provider is
[Package Cloud](https://packagecloud.io).

Netdata is committed to support installation of our solution to all operating systems. This is a constant battle for
Netdata, as we strive to automate and make things easier for our users. For the operating system support matrix, please
visit our [distributions](../../DISTRIBUTIONS.md) support page.

We provide two separate repositories, one for our stable releases and one for our nightly releases.

1.  Stable releases: Our stable production releases are hosted in
    [netdata/netdata](https://packagecloud.io/netdata/netdata) repository of package cloud
2.  Nightly releases: Our latest releases are hosted in
    [netdata/netdata-edge](https://packagecloud.io/netdata/netdata-edge) repository of package cloud

Visit the repository pages and follow the quick set-up instructions to get started.

## What's next?

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs.

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../../docs/getting-started.md) for a quick overview.
