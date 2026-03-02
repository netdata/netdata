# Platform support policy

Netdata is evolving rapidly and new features are added at a constant pace. Therefore, we have a frequent release cadence to deliver all these features to you as soon as possible.

You can choose from two Netdata Agent release channels:

| Release Channel |                            Release Frequency                            |                                 Support Policy & Features                                  |                                                                   Support Duration                                                                   |                              Backwards Compatibility                              |
|:---------------:|:-----------------------------------------------------------------------:|:------------------------------------------------------------------------------------------:|:----------------------------------------------------------------------------------------------------------------------------------------------------:|:---------------------------------------------------------------------------------:|
|   **Stable**    | Usually 4-6 major/minor releases per year plus patch releases as needed |                 Receiving bug fixes and security updates between releases                  |                                                       Up to the 2nd stable release after them                                                        |     Previous configuration semantics and data are supported by newer releases     |
|   **Nightly**   |                      Most nights around 02:00 UTC                       | • Receiving bug fixes and security updates continuously<br/>• Latest pre-released features | • Issues are generally investigated against recent nightlies<br/>• Users may be asked to reproduce on a current build before further troubleshooting | Configuration and data of unreleased features may change between nightly releases |

:::info

"Support Duration" defines how long we consider each release actively used in production systems. After this period, you should update to the latest release to continue receiving bug fixes and security updates.

:::

:::tip

**Switching Between Stable and Nightly Builds**: You can switch between stable and nightly channels depending on your needs. The method depends on how you originally installed Netdata. For further information, please reference our [switching guide](https://learn.netdata.cloud/docs/netdata-agent/installation/linux/switch-install-types-and-release-channels).

:::

Netdata defines three tiers of official support:

- [Core](#core)
- [Intermediate](#intermediate)
- [Community](#community)

Each tier defines different guarantees for platforms in that tier, described below in the section about that tier.

Additionally, we define two categories for special cases that we do not support:

- [Third-party supported platforms](#third-party-supported-platforms)
- [Previously supported platforms](#previously-supported-platforms)

These two categories are explained further below.

Any platforms not listed in any of these categories may or may not work.

The following table shows a general outline of the various support tiers and categories.

|                       | Bug Support                            | Guaranteed Configurations      | CI Coverage                                                                                       | Native Packages                          | Static Build Support |
|-----------------------|----------------------------------------|--------------------------------|---------------------------------------------------------------------------------------------------|------------------------------------------|----------------------|
| Core                  | High priority                          | Everything but rare edge cases | Full                                                                                              | Yes, if we can provide them              | Full                 |
| Intermediate          | Normal priority                        | Common cases                   | Partial (CI mostly equivalent to **Core**, but possibly with some gaps, and not required to pass) | Possibly                                 | Full                 |
| Community             | Best Effort                            | Default only                   | None                                                                                              | No                                       | Best Effort          |
| Third-party Supported | Users directed to platform maintainers | None                           | None                                                                                              | No                                       | Best Effort          |
| Previously Supported  | Users asked to upgrade                 | None                           | None                                                                                              | Yes, but only already published versions | Best Effort          |

- 'Bug Support': How we handle platform-specific bugs.
- 'Guaranteed Configurations': Which runtime configurations for the Agent we try to guarantee will work with minimal effort from users.
- 'CI Coverage': What level of coverage we provide for the platform in CI.
- 'Native Packages': Whether we provide native packages for the system package manager for the platform.
- 'Static Build Support': How well our static builds are expected to work on the platform.

## Currently supported platforms

### Core

Platforms in the core support tier are our top priority. They are covered rigorously in our CI, usually include official binary packages, and any platform-specific bugs receive a high priority. From the perspective of our developers, platforms in the core support tier _must_ work, with almost no exceptions.

Our [static builds](#static-builds) are expected to work on these platforms if available. Source-based installs are expected to work on these platforms with minimal user effort.

| Platform                 | Version        | Official Native Packages      | Notes                                                                                                          |
|--------------------------|----------------|-------------------------------|----------------------------------------------------------------------------------------------------------------|
| Alpine Linux             | 3.23           | No                            | The latest release of Alpine Linux is guaranteed to remain at **Core** tier due to usage for our Docker images |
| Alpine Linux             | 3.22           | No                            |                                                                                                                |
| Alma Linux               | 9.x            | x86\_64, AArch64              | Also includes support for Rocky Linux and other ABI compatible RHEL derivatives                                |
| Alma Linux               | 8.x            | x86\_64, AArch64              | Also includes support for Rocky Linux and other ABI compatible RHEL derivatives                                |
| Amazon Linux             | 2023           | x86\_64, AArch64              |                                                                                                                |
| Amazon Linux             | 2              | x86\_64, AArch64              |                                                                                                                |
| CentOS                   | 7.x            | x86\_64                       |                                                                                                                |
| Docker                   | 19.03 or newer | x86\_64, ARMv7, AArch64       | See our [Docker documentation](/packaging/docker/README.md) for more info on using Netdata on Docker           |
| Debian                   | 13.x           | x86\_64, i386, ARMv7, AArch64 |                                                                                                                |
| Debian                   | 12.x           | x86\_64, i386, ARMv7, AArch64 |                                                                                                                |
| Debian                   | 11.x           | x86\_64, i386, ARMv7, AArch64 |                                                                                                                |
| Fedora                   | 43             | x86\_64, AArch64              |                                                                                                                |
| Fedora                   | 42             | x86\_64, AArch64              |                                                                                                                |
| openSUSE                 | Tumbleweed     | x86\_64, AArch64              |                                                                                                                |
| openSUSE                 | Leap 16.0      | x86\_64, AArch64              |                                                                                                                |
| openSUSE                 | Leap 15.6      | x86\_64, AArch64              |                                                                                                                |
| Oracle Linux             | 10.x           | x86\_64, AArch64              |                                                                                                                |
| Oracle Linux             | 9.x            | x86\_64, AArch64              |                                                                                                                |
| Oracle Linux             | 8.x            | x86\_64, AArch64              |                                                                                                                |
| Red Hat Enterprise Linux | 9.x            | x86\_64, AArch64              |                                                                                                                |
| Red Hat Enterprise Linux | 8.x            | x86\_64, AArch64              |                                                                                                                |
| Red Hat Enterprise Linux | 7.x            | x86\_64                       |                                                                                                                |
| Rocky Linux              | 10.x           | x86\_64, AArch64              | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Rocky Linux              | 9.x            | x86\_64, AArch64              | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Rocky Linux              | 8.x            | x86\_64, AArch64              | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Ubuntu                   | 25.10          | x86\_64, AArch64, ARMv7       |                                                                                                                |
| Ubuntu                   | 24.04          | x86\_64, AArch64, ARMv7       |                                                                                                                |
| Ubuntu                   | 22.04          | x86\_64, ARMv7, AArch64       |                                                                                                                |

### Intermediate

Platforms in the intermediate support tier are those which Netdata wants to support, but cannot justify core level support for. They are also covered in CI, but not as rigorously as the core tier. They may or may not include official binary packages, and any platform-specific bugs receive a normal priority. Generally, we will add new platforms that we officially support ourselves to the intermediate tier. Our [static builds](#static-builds) are expected to work on these platforms if available. Source-based installs are expected to work on these platforms with minimal user effort.

| Platform      | Version | Official Native Packages | Notes                                                                   |
|---------------|---------|--------------------------|-------------------------------------------------------------------------|
| Alpine Linux  | Edge    | No                       |                                                                         |
| Arch Linux    | Latest  | No                       | We officially recommend the community packages available for Arch Linux |
| Manjaro Linux | Latest  | No                       | We officially recommend the community packages available for Arch Linux |

### Community

Platforms in the community support tier are those which are primarily supported by community contributors. They may receive some support from Netdata, but are only a best-effort affair. When a community member makes a contribution to add support for a new platform, that platform generally will start in this tier. Our [static builds](#static-builds) are expected to work on these platforms if available. Source-based installs are usually expected to work on these platforms, but may require some extra effort from users.

| Platform    | Version   | Official Native Packages | Notes                                                                                                     |
|-------------|-----------|--------------------------|-----------------------------------------------------------------------------------------------------------|
| Clear Linux | Latest    | No                       |                                                                                                           |
| Debian      | Sid       | No                       |                                                                                                           |
| Fedora      | Rawhide   | No                       |                                                                                                           |
| FreeBSD     | 13-STABLE | No                       | Netdata is included in the FreeBSD Ports Tree, and this is the recommended installation method on FreeBSD |
| Gentoo      | Latest    | No                       |                                                                                                           |
| macOS       | 13        | No                       | Currently only works for Intel-based hardware. Requires Homebrew for dependencies                         |
| macOS       | 12        | No                       | Currently only works for Intel-based hardware. Requires Homebrew for dependencies                         |
| macOS       | 11        | No                       | Currently only works for Intel-based hardware. Requires Homebrew for dependencies.                        |

## Binary Distribution Packages

The tier tables above are the authoritative source for supported platform versions and architectures.

For quick format guidance:

- Docker under Linux is distributed as a `docker image`.
- Static builds are distributed as `.gz.run` installers.
- Linux native packages are distributed as `DEB` and `RPM` for platforms in the Core tier where native packages are available.

:::important

Linux distributions frequently provide binary packages of Netdata. However, **the packages you will find in the distributions' repositories may be outdated, incomplete, missing significant features, or completely broken**. We recommend using the packages we provide.

:::

## Builds from Source

Source builds are expected to work for all platforms in the Core tier because they are continuously validated in CI. The following additional environments should usually work, although we do not continuously validate all edge cases:

|            Platform            | Platform Versions |
|:------------------------------:|:-----------------:|
|    **Linux Distributions**     | Latest unreleased |
|  **FreeBSD and derivatives**   |     13-STABLE     |
|   **Gentoo and derivatives**   |      Latest       |
| **Arch Linux and derivatives** |  Latest from AUR  |
|           **macOS**            |    11, 12, 13     |

## Third-party supported platforms

The following distributions are maintained by third parties and generally provide recent Netdata packages:

|    Platform    | Platform Versions |    Released Packages Architecture    |
|:--------------:|:-----------------:|:------------------------------------:|
| **Arch Linux** |      Latest       | All the Arch-supported architectures |

Some platform maintainers actively support Netdata on their platforms even though we do not provide official support. Third-party supported platforms may work, but the experience of using Netdata on such platforms is not something we can guarantee. When you use an externally supported platform and report a bug, we will either ask you to reproduce the issue on a supported platform or submit a support request directly to the platform maintainers.

Currently, we know of the following platforms having some degree of third-party support for Netdata:

- NixOS: Netdata's official installation methods do not support NixOS, but the NixOS maintainers provide their own Netdata packages for their platform.
- Rockstor: Rockstor provides support for a Netdata add-on for their NAS platform. The Rockstor community and developers are the primary source for support on their platform.

## Previously supported platforms

:::note

We consider a platform to be end of life when the upstream maintainers of that platform stop providing official support for it themselves, or when that platform transitions into an 'extended security maintenance' period.

Platforms that meet these criteria will be immediately transitioned to the **Previously Supported** category. Unlike platforms dropped for technical reasons, there will be no prior warning from Netdata and no deprecation notice. This is because our end of support should already coincide with the end of the normal support lifecycle for that platform.

:::

This is a list of platforms that we have supported in the recent past but no longer officially support:

| Platform | Version   | Notes                |
|----------|-----------|----------------------|
| Debian   | 10.x      | EOL as of 2024-07-01 |
| Fedora   | 41        | EOL as of 2025-12-15 |
| Fedora   | 40        | EOL as of 2024-11-12 |
| openSUSE | Leap 15.5 | EOL as of 2024-12-31 |
| Ubuntu   | 25.04     | EOL as of 2026-01-17 |
| Ubuntu   | 20.04     | EOL as of 2025-05-31 |
| Ubuntu   | 18.04     | EOL as of 2023-04-02 |

## Static builds

The Netdata team provides static builds of Netdata for Linux systems with a selection of common CPU architectures. These static builds are largely self-contained, only requiring a POSIX-compliant shell on the target system to provide their basic functionality. Static builds are built in an Alpine Linux environment using musl. This means that they generally do not support non-local username mappings or exotic name resolution configurations.

### Build Process

The entrypoint for the static build process is `packaging/makeself/build-static.sh`. This script uses Docker or Podman to create self-contained static binaries for multiple architectures. The build system uses QEMU emulation to enable cross-architecture builds on x86_64 build hosts.

### Supported Architectures

We currently provide static builds for the following CPU architectures:

- 64-bit x86 (x86_64/amd64)
- ARMv7 (armv7l)
- ARMv6 (armv6l)
- AArch64 (arm64)

### Limitations of Static Builds

When you use static builds, you'll miss certain features that require specific operating system support, including:

- IPMI hardware sensors monitoring
- eBPF-related capabilities

### Systemd

Many of our systemd integrations are not supported in our static builds. This is due to a general refusal by the systemd developers to support static linking (or any C runtime other than glibc), and is not something we can resolve.

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
