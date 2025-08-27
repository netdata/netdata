---
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/netdata-agent/versions-and-platforms.md"
sidebar_label: "Versions & Platforms"
learn_status: "Published"
learn_rel_path: "Netdata Agent"
description: "Present all the supported platform in the Netdata solution"
sidebar_position: "600010"
learn_link: "https://learn.netdata.cloud/docs/netdata-agent/versions-&-platforms"
---

# Netdata Agent Versions & Platforms

Netdata is evolving rapidly and new features are added at a constant pace. Therefore, we have a frequent release cadence to deliver all these features to you as soon as possible.

You can choose from 2 Netdata Agent release channels:

| Release Channel |                            Release Frequency                            |                 Support Policy & Features                 |             Support Duration             |                              Backwards Compatibility                              |
| :-------------: | :---------------------------------------------------------------------: | :-------------------------------------------------------: | :--------------------------------------: | :-------------------------------------------------------------------------------: |
|   **Stable**    | Usually 4-6 major/minor releases per year plus patch releases as needed | Receiving bug fixes and security updates between releases | Up to the 2nd stable release after them  |     Previous configuration semantics and data are supported by newer releases     |
|   **Nightly**   |                      Most nights around 02:00 UTC                       |               Latest pre-released features                | Up to the 2nd nightly release after them | Configuration and data of unreleased features may change between nightly releases |

:::info

"Support Duration" defines how long we consider each release actively used in production systems. After this period, you should update to the latest release to continue receiving bug fixes and security updates.

:::

:::tip

**Switching Between Stable and Nightly Builds**: You can switch between stable and nightly channels depending on your needs. The method depends on how you originally installed Netdata. For further information, please reference our [switching guide](https://learn.netdata.cloud/docs/netdata-agent/installation/linux/switch-install-types-and-release-channels).

:::

## Binary Distribution Packages

We provide binary distribution packages via CI integration for the following platforms and architectures:

|          Platform           |     Platform Versions      |    Released Packages Architecture     |    Format    |
| :-------------------------: | :------------------------: | :-----------------------------------: | :----------: |
|   **Docker under Linux**    |      19.03 and later       | `x86_64`, `i386`, `ARMv7`, `AArch64`  | docker image |
|      **Static Builds**      |             -              | `x86_64`, `ARMv6`, `ARMv7`, `AArch64` |   .gz.run    |
|       **Alma Linux**        |          8.x, 9.x          |          `x86_64`, `AArch64`          |     RPM      |
|      **Amazon Linux**       |          2, 2023           |          `x86_64`, `AArch64`          |     RPM      |
|         **Centos**          |            7.x             |               `x86_64`                |     RPM      |
|         **Debian**          |      10.x, 11.x, 12.x      | `x86_64`, `i386`, `ARMv7`, `AArch64`  |     DEB      |
|         **Debian**          |            13.x            |     `x86_64`, `ARMv7`, `AArch64`      |     DEB      |
|         **Fedora**          |           41, 42           |          `x86_64`, `AArch64`          |     RPM      |
|        **OpenSUSE**         |   Leap 15.6, Tumbleweed    |          `x86_64`, `AArch64`          |     RPM      |
|      **Oracle Linux**       |       8.x, 9.x, 10.x       |          `x86_64`, `AArch64`          |     RPM      |
| **Redhat Enterprise Linux** |            7.x             |               `x86_64`                |     RPM      |
| **Redhat Enterprise Linux** |          8.x, 9.x          |          `x86_64`, `AArch64`          |     RPM      |
|         **Ubuntu**          | 20.04, 22.04, 24.04, 25.04 |     `x86_64`, `ARMv7`, `AArch64`      |     DEB      |

:::important

Linux distributions frequently provide binary packages of Netdata. However, **the packages you will find in the distributions' repositories may be outdated, incomplete, missing significant features or completely broken**. We recommend using the packages we provide.

:::

## Third-party Supported Binary Packages

The following distributions always provide the latest stable version of Netdata:

|    Platform    | Platform Versions |    Released Packages Architecture    |
| :------------: | :---------------: | :----------------------------------: |
| **Arch Linux** |      Latest       | All the Arch supported architectures |

## Builds from Source

We guarantee that you can build Netdata from source for the platforms where we provide automated binary packages. These platforms are automatically checked via our CI, and fixes are always applied to allow merging new code into the nightly versions.

The following builds from source should usually work for you, although we don't regularly monitor if there are issues:

|            Platform            |     Platform Versions      |
| :----------------------------: | :------------------------: |
|    **Linux Distributions**     | Latest unreleased versions |
|  **FreeBSD and derivatives**   |         14-STABLE          |
|   **Gentoo and derivatives**   |           Latest           |
| **Arch Linux and derivatives** |      latest from AUR       |
|           **MacOS**            |         13, 14, 15         |

## Static Builds and Unsupported Linux Versions

You can run Netdata's static builds on any Linux platform with a supported CPU architecture, requiring only a kernel version of 2.6 or newer. These self-contained packages include everything you need for Netdata to operate effectively.

### Limitations of Static Builds

When you use static builds, you'll miss certain features that require specific operating system support, including:

- IPMI hardware sensors monitoring
- eBPF-related capabilities

### Impact of Platform End-of-Life (EOL)

When a platform is removed from the Binary Distribution Packages list:

- **No automatic transitions occur**: Your existing native package installations will remain as they are
- **Your local updater will report the Agent as up-to-date** even when newer versions exist.
- **When a new Netdata version is published, you'll see** "_Nodes are below the recommended Agent version_" **warnings** in the Netdata Cloud UI.
- **You will stop receiving new features, improvements, and security updates**.

:::important

**We strongly recommend upgrading your operating system before it reaches EOL** to maintain full Netdata functionality and continued updates.

:::

:::tip

**Migrating from Native Package to Static Build**: If upgrading your operating system isn't possible, you can manually switch to a static build. For more information, please reference our [switching guide](https://learn.netdata.cloud/docs/netdata-agent/installation/linux/switch-install-types-and-release-channels).

:::
