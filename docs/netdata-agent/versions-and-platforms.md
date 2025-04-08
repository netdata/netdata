# Netdata Agent Versions & Platforms

Netdata is evolving rapidly and new features are added at a constant pace. Therefore, we have a frequent release cadence to deliver all these features to use as soon as possible.

Netdata Agents are available in 2 versions:

| Release Channel |               Release Frequency               |                 Support Policy & Features                 |             Support Duration             |                              Backwards Compatibility                              |
|:---------------:|:---------------------------------------------:|:---------------------------------------------------------:|:----------------------------------------:|:---------------------------------------------------------------------------------:|
|     Stable      | At most once per month, usually every 45 days | Receiving bug fixes and security updates between releases | Up to the 2nd stable release after them  |     Previous configuration semantics and data are supported by newer releases     |
|     Nightly     |           Every night at 00:00 UTC            |               Latest pre-released features                | Up to the 2nd nightly release after them | Configuration and data of unreleased features may change between nightly releases |

> "Support Duration" defines the time we consider the release as actively used by users in production systems, so that all features of Netdata should be working like the day they were released. However, after the latest release, previous releases stop receiving bug fixes and security updates. All users are advised to update to the latest release to get the latest bug fixes.

## Binary Distribution Packages

Binary distribution packages are provided by Netdata, via CI integration, for the following platforms and architectures:

|        Platform         |        Platform Versions         |          Released Packages Architecture          |    Format    |
|:-----------------------:|:--------------------------------:|:------------------------------------------------:|:------------:|
|   Docker under Linux    |         19.03 and later          | `x86_64`, `i386`, `ARMv7`, `AArch64`, `POWER8+`  | docker image |
|      Static Builds      |                -                 | `x86_64`, `ARMv6`, `ARMv7`, `AArch64`, `POWER8+` |   .gz.run    |
|       Alma Linux        |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
|      Amazon Linux       |             2, 2023              |               `x86_64`, `AArch64`                |     RPM      |
|         Centos          |               7.x                |                     `x86_64`                     |     RPM      |
|         Debian          |         10.x, 11.x, 12.x         |       `x86_64`, `i386`, `ARMv7`, `AArch64`       |     DEB      |
|         Fedora          |            37, 38, 39            |               `x86_64`, `AArch64`                |     RPM      |
|        OpenSUSE         | Leap 15.4, Leap 15.5, Tumbleweed |               `x86_64`, `AArch64`                |     RPM      |
|      Oracle Linux       |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
| Redhat Enterprise Linux |               7.x                |                     `x86_64`                     |     RPM      |
| Redhat Enterprise Linux |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
|         Ubuntu          |       20.04, 22.04, 23.10        |       `x86_64`, `i386`, `ARMv7`, `AArch64`       |     DEB      |

> IMPORTANT: Linux distributions frequently provide binary packages of Netdata. However, the packages you will find in the distributions' repositories may be outdated, incomplete, missing significant features or completely broken. We recommend using the packages we provide.

## Third-party Supported Binary Packages

The following distributions always provide the latest stable version of Netdata:

|  Platform  | Platform Versions |    Released Packages Architecture    |
|:----------:|:-----------------:|:------------------------------------:|
| Arch Linux |      Latest       | All the Arch supported architectures |
| MacOS Brew |      Latest       | All the Brew supported architectures |

## Builds from Source

We guarantee Netdata builds from source for the platforms we provide automated binary packages. These platforms are automatically checked via our CI, and fixes are always applied to allow merging new code into the nightly versions.

The following builds from source should usually work, although we don't regularly monitor if there are issues:

|              Platform               |     Platform Versions      |
|:-----------------------------------:|:--------------------------:|
|         Linux Distributions         | Latest unreleased versions |
|       FreeBSD and derivatives       |         13-STABLE          |
|       Gentoo and derivatives        |           Latest           |
|     Arch Linux and derivatives      |      latest from AUR       |
|                MacOS                |         11, 12, 13         |
| Linux under Microsoft Windows (WSL) |           Latest           |

## Static Builds and Unsupported Linux Versions

The static builds of Netdata can be used on any Linux platform of the supported architectures. The only requirement these static builds have is a working Linux kernel, any version. Everything else required for Netdata to run is inside the package itself.

Static builds usually miss certain features that require operating-system support and can’t be provided generically. These features include:

- IPMI hardware sensors support
- systemd-journal features
- eBPF related features

When platforms are removed from the [Binary Distribution Packages](/packaging/makeself/README.md) list, they default to install or update Netdata to a static build. This may mean that after platforms become EOL, Netdata on them may lose some of its features. We recommend upgrading the operating system before it becomes EOL, to continue using all the features of Netdata.

### Upgrading on an Unsupported Platform

Existing installs of native packages on a platform that is no longer supported, cannot be automatically updated at this time. The upgrader will show it is already at the newest version, because we no longer publish new packages.

If the operating system cannot be upgraded to a more recent version, the install can be switched to a static build. This is a manual process, may cause data loss, and is therefore **not supported**. However, following these steps should result in a functioning Agent with metrics data and connection to Netdata Cloud in tact:

1. Stop the Agent, [as appropriate for your platform](/docs/netdata-agent/start-stop-restart.md).
2. Backup the contents of these directories: `/etc/netdata`, `/var/cache/netdata`, `/var/lib/netdata`, `/var/log/netdata`.
3. Uninstall the native package, answering "yes" to all questions.

   ```sh
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
   ```

4. For platforms using systemd, unmask the Netdata service:

   ```sh
   sudo systemctl unmask netdata
   sudo systemctl daemon-reload
   ```

5. Install the static build:

   ```sh
   sh /tmp/netdata-kickstart.sh --static-only
   ```

6. Stop the Agent (again).
7. Copy over the data from the previous install:

   ```sh
   cd /opt/netdata
   sudo rsync -aRv --delete \
              --exclude /etc/netdata/.install-type \
              --exclude /etc/netdata/.environment \
              /etc/netdata /var/lib/netdata /var/cache/netdata /var/log/netdata ./
   ```

8. Start the Agent.
