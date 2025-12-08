# Platform support policy

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

- 'Bug Support': How we handle of platform-specific bugs.
- 'Guaranteed Configurations': Which runtime configurations for the Agent we try to guarantee will work with minimal effort from users.
- 'CI Coverage': What level of coverage we provide for the platform in CI.
- 'Native Packages': Whether we provide native packages for the system package manager for the platform.
- 'Static Build Support': How well our static builds are expected to work on the platform.

## Currently supported platforms

### Core

Platforms in the core support tier are our top priority. They are covered rigorously in our CI, usually include official binary packages, and any platform-specific bugs receive a high priority. From the perspective of our developers, platforms in the core support tier _must_ work, with almost no exceptions.

Our [static builds](#static-builds) are expected to work on these platforms if available. Source-based installs are expected to work on these platforms with minimal user effort.

| Platform                 | Version        | Official Native Packages               | Notes                                                                                                          |
|--------------------------|----------------|----------------------------------------|----------------------------------------------------------------------------------------------------------------|
| Alpine Linux             | 3.23           | No                                     | The latest release of Alpine Linux is guaranteed to remain at **Core** tier due to usage for our Docker images |
| Alpine Linux             | 3.22           | No                                     |                                                                                                                |
| Alma Linux               | 9.x            | x86\_64, AArch64                       | Also includes support for Rocky Linux and other ABI compatible RHEL derivatives                                |
| Alma Linux               | 8.x            | x86\_64, AArch64                       | Also includes support for Rocky Linux and other ABI compatible RHEL derivatives                                |
| Amazon Linux             | 2023           | x86\_64, AArch64                       |                                                                                                                |
| Amazon Linux             | 2              | x86\_64, AArch64                       |                                                                                                                |
| CentOS                   | 7.x            | x86\_64                                |                                                                                                                |
| Docker                   | 19.03 or newer | x86\_64, ARMv7, AArch64                | See our [Docker documentation](/packaging/docker/README.md) for more info on using Netdata on Docker           |
| Debian                   | 13.x           | x86\_64, i386, ARMv7, AArch64          |                                                                                                                |
| Debian                   | 12.x           | x86\_64, i386, ARMv7, AArch64          |                                                                                                                |
| Debian                   | 11.x           | x86\_64, i386, ARMv7, AArch64          |                                                                                                                |
| Fedora                   | 43             | x86\_64, AArch64                       |                                                                                                                |
| Fedora                   | 42             | x86\_64, AArch64                       |                                                                                                                |
| Fedora                   | 41             | x86\_64, AArch64                       |                                                                                                                |
| openSUSE                 | Tumbleweed     | x86\_64, AArch64                       |                                                                                                                |
| openSUSE                 | Leap 16.0      | x86\_64, AArch64                       |                                                                                                                |
| openSUSE                 | Leap 15.6      | x86\_64, AArch64                       |                                                                                                                |
| Oracle Linux             | 10.x           | x86\_64, AArch64                       |                                                                                                                |
| Oracle Linux             | 9.x            | x86\_64, AArch64                       |                                                                                                                |
| Oracle Linux             | 8.x            | x86\_64, AArch64                       |                                                                                                                |
| Red Hat Enterprise Linux | 9.x            | x86\_64, AArch64                       |                                                                                                                |
| Red Hat Enterprise Linux | 8.x            | x86\_64, AArch64                       |                                                                                                                |
| Red Hat Enterprise Linux | 7.x            | x86\_64                                |                                                                                                                |
| Rocky Linux              | 10.x           | x86\_64, AArch64                       | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Rocky Linux              | 9.x            | x86\_64, AArch64                       | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Rocky Linux              | 8.x            | x86\_64, AArch64                       | Also includes support for Alma Linux and other ABI compatible RHEL derivatives                                 |
| Ubuntu                   | 25.10          | x86\_64, AArch64, ARMv7                |                                                                                                                |
| Ubuntu                   | 25.04          | x86\_64, AArch64, ARMv7                |                                                                                                                |
| Ubuntu                   | 24.04          | x86\_64, AArch64, ARMv7                |                                                                                                                |
| Ubuntu                   | 22.04          | x86\_64, ARMv7, AArch64                |                                                                                                                |
| Ubuntu                   | 20.04          | x86\_64, ARMv7, AArch64                |                                                                                                                |

### Intermediate

Platforms in the intermediate support tier are those which Netdata wants to support, but cannot justify core level support for. They are also covered in CI, but not as rigorously as the core tier. They may or may not include official binary packages, and any platform-specific bugs receive a normal priority. Generally, we will add new platforms that we officially support ourselves to the intermediate tier. Our [static builds](#static-builds) are expected to work on these platforms if available. Source-based installs are expected to work on these platforms with minimal user effort.

| Platform      | Version    | Official Native Packages | Notes                                                                                                |
|---------------|------------|--------------------------|------------------------------------------------------------------------------------------------------|
| Alpine Linux  | Edge       | No                       |                                                                                                      |
| Arch Linux    | Latest     | No                       | We officially recommend the community packages available for Arch Linux                              |
| Manjaro Linux | Latest     | No                       | We officially recommend the community packages available for Arch Linux                              |

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

## Third-party supported platforms

Some platform maintainers actively support Netdata on their platforms even though we do not provide official support. Third-party supported platforms may work, but the experience of using Netdata on such platforms is not something we can guarantee. When you use an externally supported platform and report a bug, we will either ask you to reproduce the issue on a supported platform or submit a support request directly to the platform maintainers.

Currently, we know of the following platforms having some degree of third-party support for Netdata:

- NixOS: Netdata's official installation methods do not support NixOS, but the NixOS maintainers provide their own Netdata packages for their platform.
- Rockstor: Rockstor provides support for a Netdata add-on for their NAS platform. The Rockstor community and developers are the primary source for support on their platform.

## Previously supported platforms

:::note

We consider a platform to be end of life when the upstream maintainers of that platform stop providing official support for it themselves, or when that platform transitions into an 'extended security maintenance' period.

Platforms that meet these criteria will be immediately transitioned to the **Previously Supported** category, with no prior warning from Netdata and no deprecation notice, unlike those being dropped for technical reasons, as our end of support should already coincide with the end of the normal support lifecycle for that platform.

:::

This is a list of platforms that we have supported in the recent past but no longer officially support:

| Platform     | Version   | Notes                |
|--------------|-----------|----------------------|
| Alpine Linux | 3.18      | EOL as of 2024-05-23 |
| Alpine Linux | 3.17      | EOL as of 2023-11-01 |
| Alpine Linux | 3.16      | EOL as of 2024-05-23 |
| Alpine Linux | 3.15      | EOL as of 2023-11-01 |
| Alpine Linux | 3.14      | EOL as of 2023-05-01 |
| Debian       | 10.x      | EOL as of 2024-07-01 |
| Fedora       | 40        | EOL as of 2024-11-12 |
| Fedora       | 39        | EOL as of 2024-05-14 |
| Fedora       | 38        | EOL as of 2024-05-14 |
| Fedora       | 37        | EOL as of 2023-12-05 |
| openSUSE     | Leap 15.5 | EOL as of 2024-12-31 |
| openSUSE     | Leap 15.4 | EOL as of 2023-12-07 |
| openSUSE     | Leap 15.3 | EOL as of 2022-12-01 |
| Ubuntu       | 24.10     | EOL as of 2024-07-01 |
| Ubuntu       | 23.10     | EOL as of 2024-07-01 |
| Ubuntu       | 23.04     | EOL as of 2024-01-20 |
| Ubuntu       | 22.10     | EOL as of 2023-07-20 |
| Ubuntu       | 18.04     | EOL as of 2023-04-02 |

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

## Platform-specific support considerations

### Systemd

Many of our systemd integrations are not supported in our static builds. This is due to a general refusal by the systemd developers to support static linking (or any C runtime other than glibc), and is not something we can resolve.
