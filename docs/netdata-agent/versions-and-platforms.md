# Netdata Agent Versions & Platforms

Netdata is evolving rapidly and new features are added at a constant pace. Therefore, we have a frequent release cadence to deliver all these features to you as soon as possible.

You can choose from 2 Netdata Agent versions:

| Release Channel |               Release Frequency               |                 Support Policy & Features                 |             Support Duration             |                              Backwards Compatibility                              |
|:---------------:|:---------------------------------------------:|:---------------------------------------------------------:|:----------------------------------------:|:---------------------------------------------------------------------------------:|
|   **Stable**    | Usually 4-6 major/minor releases per year plus patch releases as needed | Receiving bug fixes and security updates between releases | Up to the 2nd stable release after them  |     Previous configuration semantics and data are supported by newer releases     |
|   **Nightly**   |         Most nights around 02:00 UTC          |               Latest pre-released features                | Up to the 2nd nightly release after them |Configuration and data of unreleased features may change between nightly releases|

:::info  

**Support Duration** defines how long we consider each release actively used in production systems. After this period, you should update to the latest release to continue receiving bug fixes and security updates.

:::

## Switching Between Stable and Nightly Builds

You can switch between stable and nightly channels depending on your needs. The method depends on how you originally installed Netdata.

<details>
<summary><strong>For Native Package Installations</strong></summary><br/>

If you installed Netdata using our native packages (RPM, DEB), you can switch channels by updating your repository configuration:

**Method 1: Using Package Manager (Recommended)**

```bash
# Switch from nightly to stable
sudo apt install netdata-repo  # This will uninstall netdata-repo-edge automatically
# Or for RPM-based systems:
sudo yum install netdata-repo  # This will uninstall netdata-repo-edge automatically
```

**Method 2: Manual Repository Update**

1. Update your repository configuration to point to the desired channel
2. Reinstall the Netdata package using your system package manager

</details>

<br />

<details>
<summary><strong>For Kickstart Script Installations</strong></summary><br/>

If you installed using the kickstart script, switching channels is straightforward:

**Switch to Stable Channel:**
```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --stable-channel --reinstall
```

**Switch to Nightly Channel:**
```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --nightly-channel --reinstall
```

</details>

<br />

### For Static Build Installations

For static builds, you need to reinstall using the kickstart script with the appropriate channel flag:

**Switch to Stable Static Build:**
```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --static-only --stable-channel --reinstall
```

**Switch to Nightly Static Build:**
```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --static-only --nightly-channel --reinstall
```

:::note  

Notes on Switching Channels

| Consideration | Details |
|:-------------|:--------|
| **Configuration Preservation** | Your existing configuration and data are preserved when switching between channels |
| **Downtime** | There will be brief downtime during the switch as the agent restarts |
| **Nightly Considerations** | Nightly builds automatically restart every night for updates, which may trigger brief connectivity alerts in Netdata Cloud |
| **Production Recommendations** | For production environments, stable channel is recommended for better predictability |

:::

## Binary Distribution Packages

We provide binary distribution packages via CI integration for the following platforms and architectures:

|        Platform         |        Platform Versions         |          Released Packages Architecture          |    Format    |
|:-----------------------:|:--------------------------------:|:------------------------------------------------:|:------------:|
| **Docker under Linux** |         19.03 and later          | `x86_64`, `i386`, `ARMv7`, `AArch64`  | docker image |
|   **Static Builds**    |                -                 | `x86_64`, `ARMv6`, `ARMv7`, `AArch64` |   .gz.run    |
|    **Alma Linux**      |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
|   **Amazon Linux**     |             2, 2023              |               `x86_64`, `AArch64`                |     RPM      |
|      **Centos**        |               7.x                |                     `x86_64`                     |     RPM      |
|       **Debian**       |         10.x, 11.x, 12.x         |       `x86_64`, `i386`, `ARMv7`, `AArch64`       |     DEB      |
|       **Fedora**       |            37, 38, 39            |               `x86_64`, `AArch64`                |     RPM      |
|      **OpenSUSE**      | Leap 15.4, Leap 15.5, Tumbleweed |               `x86_64`, `AArch64`                |     RPM      |
|    **Oracle Linux**    |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
| **Redhat Enterprise Linux** |               7.x                |                     `x86_64`                     |     RPM      |
| **Redhat Enterprise Linux** |             8.x, 9.x             |               `x86_64`, `AArch64`                |     RPM      |
|       **Ubuntu**       |       20.04, 22.04, 23.10        |       `x86_64`, `i386`, `ARMv7`, `AArch64`       |     DEB      |

:::important  

Linux distributions frequently provide binary packages of Netdata. However, **the packages you will find in the distributions' repositories may be outdated, incomplete, missing significant features or completely broken**. We recommend using the packages we provide.

:::

## Third-party Supported Binary Packages

The following distributions always provide the latest stable version of Netdata:

|  Platform  | Platform Versions |    Released Packages Architecture    |
|:----------:|:-----------------:|:------------------------------------:|
| **Arch Linux** |      Latest       | All the Arch supported architectures |

## Builds from Source

We guarantee that you can build Netdata from source for the platforms where we provide automated binary packages. These platforms are automatically checked via our CI, and fixes are always applied to allow merging new code into the nightly versions.

The following builds from source should usually work for you, although we don't regularly monitor if there are issues:

|              Platform               |     Platform Versions      |
|:-----------------------------------:|:--------------------------:|
|      **Linux Distributions**       | Latest unreleased versions |
|    **FreeBSD and derivatives**     |         14-STABLE          |
|     **Gentoo and derivatives**     |           Latest           |
|   **Arch Linux and derivatives**   |      latest from AUR       |
|             **MacOS**               |         13, 14, 15         |

## Static Builds and Unsupported Linux Versions

You can run Netdata's static builds on any Linux platform with supported architecture, requiring only a functioning Linux kernel of any version. These self-contained packages include everything you need for Netdata to operate effectively.

### Limitations of Static Builds

When you use static builds, you'll miss certain features that require specific operating system support, including:

- IPMI hardware sensors monitoring
- systemd-journal functionality
- eBPF-related capabilities

### Impact of Platform End-of-Life (EOL)

When a platform is removed from the Binary Distribution Packages list:

- **No automatic transitions occur**: Your existing native package installations will remain as they are
- **Your local updater will report the Agent as up-to-date** even when newer versions exist.
- **When a new Netdata version is published, you'll see** "*Nodes are below the recommended Agent version*" **warnings** in the Netdata Cloud UI.
- **You will stop receiving new features, improvements, and security updates**.

:::important  

**We strongly recommend upgrading your operating system before it reaches EOL** to maintain full Netdata functionality and continued updates.

:::

### Migrating from Native Package to Static Build: Step-by-Step Guide

If upgrading your operating system isn't possible, you can manually switch to a static build. 

:::important

This process is **not officially supported** and may result in data loss. However, following these steps should preserve your metrics data and Netdata Cloud connection:

:::

1. Stop your Netdata Agent [using the appropriate method for your platform](/docs/netdata-agent/start-stop-restart.md).

2. Back up your Netdata configuration and data: `/etc/netdata`, `/var/cache/netdata`, `/var/lib/netdata`, `/var/log/netdata`.

3. Uninstall the native package (confirm all prompts with "yes").

   <details>
   <summary>Click to see command</summary><br/>

   ```bash
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
   ```

   </details>

4. For systemd-based platforms, unmask the Netdata service:

   <details>
   <summary>Click to see command</summary><br/>

   ```bash
   sudo systemctl unmask netdata
   sudo systemctl daemon-reload
   ```

   </details>

5. Install the static build:

   <details>
   <summary>Click to see command</summary><br/>

   ```bash
   # For nightly builds
   sh /tmp/netdata-kickstart.sh --static-only --dont-start-it

   # For stable release builds
   sh /tmp/netdata-kickstart.sh --static-only --dont-start-it --stable-channel
   ```

   </details>

6. Restore your data from the previous installation:

   <details>
   <summary>Click to see command</summary><br/>

   ```bash
   # Install rsync if needed (example for Debian/Ubuntu)
   # sudo apt-get update && sudo apt-get install -y rsync
   # For RHEL/CentOS/Fedora
   # sudo yum install -y rsync

   cd /opt/netdata
   sudo rsync -aRv --delete \
              --exclude /etc/netdata/.install-type \
              --exclude /etc/netdata/.environment \
              /etc/netdata /var/lib/netdata /var/cache/netdata /var/log/netdata ./
   ```

   </details>

7. Start your Netdata Agent to complete the migration.