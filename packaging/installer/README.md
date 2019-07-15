# Installation

You can install Netdata in many different ways, but a few methods will be ideal for the vast majority of users:

- [**Automatic one-line installation**](#one-line-installation) (**recommended**): Easy, fully automated installation directly from Netdata's source code
- [Binary packages via Netdata's repository](#binary-packages): Use your distribution's package manager (RPM-based distributions only) to install Netdata
- [Docker container](../docker/): Use our `Dockerfile` to test Netdata or install it in an existing Docker-based infrastructure

To see whether Netdata supports your system, please visit our [distribution support matrix](../../packaging/DISTRIBUTIONS.md).

!!! note
    Starting with Netdata v1.12, Netdata by default collects anonymous usage information and sends it to Google Analytics. To read more about what information is collected and how to opt-out, check the [anonymous statistics page](../../docs/anonymous-statistics.md).

**Other installation methods for MacOS, FreeBSD, Kubernetes, and more:** 

- [Pre-built static binary](OTHERS.md): An automated installation process for any Intel/AMD 64bit Linux system
- [Manual installation](MANUAL-INSTALLATION.md)
- [FreeBSD](OTHERS.md#freebsd)
- [pfSense](OTHERS.md#pfsense)
- [FreeNAS Corral](OTHERS.md#freenas)
- [macOS (OS X)](OTHERS.md#macos)
- [Kubernetes cluster](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)
- See the list of Netdata [package maintainers](../maintainers) for ASUSTOR NAS, OpenWRT, ReadyNAS, and other niche systems.

---

## One-line installation 
![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is **fully automatic on all Linux distributions**. FreeBSD and MacOS systems need some preparations before installing Netdata for the first time. Check the [FreeBSD](OTHERS.md#freebsd) and the [MacOS](OTHERS.md#macos) sections for more information.

To install Netdata from source, and keep it up to date with our **nightly releases** automatically, run the following: 

```bash
$ bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

!!! note
    Do not use `sudo` for the one-line installer—it will escalate privileges itself if needed.

    To learn more about the pros and cons of using *nightly* vs. *stable* releases, see our [notice about the two options](#nightly-vs-stable-releases).

Once you have installed Netdata, see our [getting started guide](../../docs/GettingStarted.md).

<details markdown="1"><summary>More information and advanced uses of the `kickstart.sh` script</summary>

**What `kickstart.sh` does:**

The `kickstart.sh` script:

- Detects the Linux distro and installs the required system packages for building Netdata after asking for confirmation
- Downloads the latest Netdata source tree to `/usr/src/netdata.git`
- Installs Netdata by running `./netdata-installer.sh` from the source tree
- Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated daily
- Outputs details about whether the installation succeeded or failed.

**Available options:**

You can customize your Netdata installation by passing options from `kickstart.sh` to `netdata-installer.sh`. With these options you can change the installation directory, enable/disable automatic updates, choose between the nightly (default) or stable channel, enable/disable plugins, and much more. For a full list of options, see the [`netdata-installer.sh` script](https://github.com/netdata/netdata/netdata-installer.sh#L149-L177).

Here are a few popular options:

- `--stable-channel`: Automatically update only on the release of new major versions.
- `--no-updates`: Prevent automatic updates of any kind.
- `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
- `--dont-start-it`: Prevent the installer from starting Netdata automatically.

Here's an example of how to pass a few options through `kickstart.sh`:

```bash
  bash <(curl -Ss https://my-netdata.io/kickstart.sh) --dont-wait --dont-start-it --stable-channel
```

**Verify the script's integrity:**

Verify the integrity of the script with this:

```bash
[ "8a2b054081a108dff915994ce77f2f2d" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

This command will output `OK, VALID` to confirm that the script is intact and has not been tampered with.

</details>

## Binary packages 
![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

In the effort to make Netdata's installation process easy and fully automated on as many operating systems as possible, we have begun providing our own binary packages for the most common Linux distributions that use `.RPM` or `.DEB` packaging formats.

**We have currently only released `.RPM` packages**. `.DEB` packages will be available in an upcoming release. 

We provide two separate repositories, one for our stable releases and one for our nightly releases. Our current packaging infrastructure provider is [Package Cloud](https://packagecloud.io). You can visit the repository pages to read more, or try the set-up commands to get started.

**Stable releases:** Our stable production releases are hosted in the [netdata/netdata](https://packagecloud.io/netdata/netdata) repository on Packagecloud.

Use the following command to add our stable repository to your system's package manager:

```
curl -s https://packagecloud.io/install/repositories/netdata/netdata/script.rpm.sh | sudo bash
```

**Nightly releases:** Our latest releases are hosted in the [netdata/netdata-edge](https://packagecloud.io/netdata/netdata-edge) repository on Packagecloud.

Use the following command to add our nightly repository to your system's package manager:

```
curl -s https://packagecloud.io/install/repositories/netdata/netdata-edge/script.rpm.sh | sudo bash
```

## Nightly vs. stable releases

The Netdata team maintains two releases of the Netdata agent: **nightly** and **stable**. By default, Netdata's installation scripts will give you **automatic, nightly** updates, as that is our recommended configuration.

**Nightly**: We create nightly builds every 24 hours. They contain fully-tested code that fixes bugs or security flaws, or introduces new features to Netdata. Every nightly release is a candidate for then becoming a stable release—when we're ready, we simply change the release tags on GitHub. That means nightly releases are stable and proven to function correctly in the vast majority of Netdata use cases. That's why nightly is the *best choice for most Netdata users*.

**Stable**: We create stable releases whenever we believe the code has reached a major milestone. Most often, stable releases correlate with the introduction of new, significant features. Stable releases might be a better choice for those who run Netdata in *mission-critical production systems*, as updates will come more infrequently, and only after the community helps fix any bugs that might have been introduced in previous releases.

**Pros of using nightly releases:**

  - Get the latest features and bugfixes as soon as they're available
  - Receive security-related fixes immediately
  - Use stable, fully-tested code that's always improving
  - Leverage the same Netdata experience our community is using

**Pros of using stable releases:**

  - Protect yourself from the rare instance when major bugs slip through our testing and negatively affect a Netdata installation
  - Retain more control over the Netdata version you use


## Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to manually update your Netdata agent, you can disable automatic updates by using the `--no-updates` option when you install or update Netdata using the [one-line installation script](#one-line-installation).

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update Netdata](UPDATE.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
