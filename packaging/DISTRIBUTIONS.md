# Netdata distribution support matrix

![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

In the following table we've listed Netdata's official supported operating systems. We detail the distributions, flavors, and the level of support Netdata is currently capable to provide.

The following table is a work in progress. We have concluded on the list of distributions
that we currently supporting and we are working on documenting our current state so that our users
have complete visibility over the range of support.

**Legend**:

-   **Version**: Operating system version supported
-   **Family**: The family that the OS belongs to
-   **CI: Smoke Testing**: Smoke testing has been implemented on our CI, to prevent broken code reaching our users
-   **CI: Testing**: Testing has been implemented to prevent broken or problematic code reaching our users
-   **CD**: Continious deployment support has been fully enabled for this operating system
-   **.DEB**: We provide a `.DEB` package for that particular operating system
-   **.RPM**: We provide a `.RPM` package for that particular operating system
-   **Installer**: Running netdata from source, using our installer, is working for this operating system
-   **Kickstart**: Kickstart installation is working fine for this operating system
-   **Kickstart64**: Kickstart static64 installation is working fine for this operating system
-   **Community**: This operating system receives community support, such as packaging maintainers, contributors, and so on

## AMD64 Architecture

| Version | Family | CI: Smoke testing | CI: Testing | CD | .DEB | .RPM | Installer | Kickstart | Kickstart64 | Community 
:------------------: | :------------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------:
| 14.04.6 LTS (Trusty Tahr) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 16.04.6 LTS (Xenial Xerus) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 18.04.2 LTS (Bionic Beaver) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 19.04 (Disco Dingo) Latest | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 7 (Wheezy) | Debian | &#10004; | &#63; | &#10004; | &#10007; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 8 (Jessie) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 9 (Stretch) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 10 (Buster) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Versions 6.* | RHEL |  &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Versions 7.* | RHEL | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Versions 8.* | RHEL |  &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| Fedora 28 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 29 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 30 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 31 | Fedora | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| CentOS 6.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| CentOS 7.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| CentOS 8.* | Cent OS | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| OpenSuSE Leap 15.0 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| OpenSuSE Leap 15.1 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| OpenSuSE Tumbleweed | Open SuSE | &#10004; | &#63; | &#63; | N/A | &#10007; | &#10004; | &#63; | &#10004; | &#63;
| SLES 11 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| SLES 12 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| SLES 15 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| Alpine | Alpine | &#10004; | &#63; | &#10007; | N/A | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Arch Linux (latest) | Arch | &#10004; | &#63; | &#10007; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| All other linux | Other | &#63; | &#63; | &#63; | &#10007; | &#10007; | &#63; | &#63; | &#10004; | &#63;

## x86 Architecture

| Version | Family | CI: Smoke testing | CI: Testing | CD | .DEB | .RPM | Installer | Kickstart | Kickstart64 | Community 
:------------------: | :------------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------:
| 14.04.6 LTS (Trusty Tahr) | Ubuntu | &#10004; | &#63; | &#10004; | &#10007; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 16.04.6 LTS (Xenial Xerus) | Ubuntu | &#10004; | &#63; | &#10004; | &#10007; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 18.04.2 LTS (Bionic Beaver) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| 19.04 (Disco Dingo) Latest | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 7 (Wheezy) | Debian | &#10004; | &#63; | &#10004; | &#10007; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 8 (Jessie) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 9 (Stretch) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Debian 10 (Buster) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Versions 6.* | RHEL |  &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Versions 7.* | RHEL | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Versions 8.* | RHEL |  &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| Fedora 28 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 29 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 30 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| Fedora 31 | Fedora | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| CentOS 6.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| CentOS 7.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| CentOS 8.* | Cent OS | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| OpenSuSE Leap 15.0 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| OpenSuSE Leap 15.1 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
| OpenSuSE Tumbleweed | Open SuSE | &#10004; | &#63; | &#63; | N/A | &#10007; | &#10004; | &#63; | &#10004; | &#63;
| SLES 11 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| SLES 12 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| SLES 15 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#10004; | &#63;
| Alpine | Alpine | &#10004; | &#63; | &#10007; | N/A | N/A | &#10004; | &#10004; | &#10004; | &#63;
| Arch Linux (latest) | Arch | &#10004; | &#63; | &#10007; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
| All other linux | Other | &#63; | &#63; | &#63; | &#10007; | &#10007; | &#63; | &#63; | &#10004; | &#63;

## Supported functionalities accross different distribution channels

On the following section we try to depict what functionalities are available, across the different distribution channels.
There are various limitations and problems we try to attend as we evolve and grow. Through this report we want to provide some clarity as to what is available and in what way. Of course we strive to deliver our full solution through all channels, but that may not be feasible yet for some cases.

**Legend**:

-   **Auto-detect**: Depends on the programs package dependencies. If the required dependencies are covered during compile time, capability is enabled
-   **YES**: This flag implies that the functionality is available for that distribution channel                
-   **NO**: Not available at the moment for that distribution channel at this time, but may be a work-in-progress effort from the Netdata team.
-   **At Runtime**: The given module or functionality is available and only requires configuration after install to enable it                

### Core functionality

#### Core

This is the base netdata capability, that includes basic monitoring, embedded web server, and so on.

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|YES|YES|YES|YES|YES|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: None
-   **What packages required for auto-detect?**: `install-required-packages.sh netdata`

#### DB Engine

This is the brand new database engine capability of netdata. It is a mandatory facility required by netdata. Given it's special needs and dependencies though, it remains an optional facility so that users can enjoy netdata even when they cannot cover the dependencies or the H/W requirements.

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|YES|YES|YES|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: `--disable-dbengine`
-   **What packages required for auto-detect?**: `openssl`, `libuv1`, `lz4`, `Judy`

#### Encryption Support (HTTPS)

This is Netdata's TLS capability that incorporates encryption on the web server and the APIs between master and slaves. Also a mandatory facility for Netdata, but remains optional for users who are limited or not interested in tight security

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|YES|YES|YES|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-https
-   **What packages required for auto-detect?**: `openssl`

### Libraries/optimizations

#### JSON-C Support

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-jsonc
-   **What packages required for auto-detect?**: `json-c`

#### Link time optimizations

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|Auto-detect|Auto-detect|Auto-detect|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-lto
-   **What packages required for auto-detect?**: No package dependency, depends on GCC version

### External plugins, built with netdata build tools

#### FREEIPMI

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|No|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-plugin-freeipmi
-   **What packages required for auto-detect?**: `freeipmi-dev (or -devel)`

#### NFACCT

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-plugin-nfacct
-   **What packages required for auto-detect?**: `libmnl-dev`, `libnetfilter_acct-dev`

#### Xenstat

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|NO|NO|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-plugin-xenstat
-   **What packages required for auto-detect?**: `xen-dom0-libs-devel or xen-devel`, `yajl-dev or yajl-devel`
    Note: for cent-OS based systems you will need `centos-release-xen` repository to get xen-devel

#### CUPS

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-plugin-cups
-   **What packages required for auto-detect?**: `cups-devel`

#### FPING

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|

-   **Flags/instructions to enable**: ${INSTALL_PATH}/netdata/plugins.d/fping.plugin install
-   **Flags to disable from source**: None -- just dont install
-   **What packages required for auto-detect?**: None - only fping installed to start it up

#### IOPING

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|

-   **Flags/instructions to enable**: ${INSTALL_PATH}/netdata/plugins.d/ioping.plugin install
-   **Flags to disable from source**: None -- just dont install
-   **What packages required for auto-detect?**: None - only ioping installed to start it up

#### PERF

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|At Runtime|

-   **Flags/instructions to enable**: Inside netdata.conf, section `[Plugins]`, set `"perf = yes"`
-   **Flags to disable from source**: --disable-perf
-   **What packages required for auto-detect?**: None

### Backends

#### Prometheus remote write

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|YES|YES|

-   **Flags/instructions to enable**: None
-   **Flags to disable from source**: --disable-backend-prometheus-remote-write
-   **What packages required for auto-detect?**: `snappy-devel`, `protobuf`, `protobuf-compiler`

#### AWS Kinesis

|make/make install|netdata-installer.sh|kickstart.sh|kickstart-static64.sh|Docker image|RPM packaging|DEB packaging|
|:---------------:|:------------------:|:----------:|:-------------------:|:----------:|:-----------:|:-----------:|
|Auto-detect|Auto-detect|Auto-detect|Auto-detect|NO|NO|NO|

-   **Flags/instructions to enable**: [Instructions for AWS Kinesis](https://docs.netdata.cloud/backends/aws_kinesis)
-   **Flags to disable from source**: --disable-backend-kinesis
-   **What packages required for auto-detect?**: `AWS SDK for C++`, `libcurl`, `libssl`, `libcrypto`
