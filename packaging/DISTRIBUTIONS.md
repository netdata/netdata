# Netdata distribution support matrix
![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

In the following table we've listed Netdata's official supported operating systems. We detail the distributions, flavors, and the level of support Netdata is currently capable to provide.

The following table is a work in progress. We have concluded on the list of distributions
that we currently supporting and we are working on documenting our current state so that our users
have complete visibility over the range of support.

## AMD64 Architecture

Version | Family | CI Smoke testing | CI Testing | CD | .DEB | .RPM | Installer | Kickstart | Kickstart static64 | Community support
:------------------: | :------------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------: | :----------------:
14.04.6 LTS (Trusty Tahr) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
16.04.6 LTS (Xenial Xerus) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
18.04.2 LTS (Bionic Beaver) | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
19.04 (Disco Dingo) Latest | Ubuntu | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
Debian 7 (Wheezy) | Debian | &#10004; | &#63; | &#10004; | &#10007; | N/A | &#10004; | &#10004; | &#10004; | &#63;
Debian 8 (Jessie) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
Debian 9 (Stretch) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
Debian 10 (Buster) | Debian | &#10004; | &#63; | &#10004; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#63;
Versions 6.* | RHEL |  &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
Versions 7.* | RHEL | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
Versions 8.* | RHEL |  &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
Fedora 28 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
Fedora 29 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
Fedora 30 | Fedora | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
Fedora 31 | Fedora | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
CentOS 6.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
CentOS 7.* | Cent OS | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
CentOS 8.* | Cent OS | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
OpenSuSE Leap 15.0 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
OpenSuSE Leap 15.1 | Open SuSE | &#10004; | &#63; | &#10004; | N/A | &#10004; | &#10004; | &#10004; | &#10004; | &#63;
OpenSuSE Tumbleweed | Open SuSE | &#10004; | &#63; | &#63; | N/A | &#10007; | &#10004; | &#63; | &#63; | &#63;
SLES 11 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
SLES 12 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
SLES 15 | SLES | &#63; | &#63; | &#63; | N/A | &#10007; | &#63; | &#63; | &#63; | &#63;
Alpine | Alpine | &#10004; | &#63; | &#10007; | N/A | N/A | &#10004; | &#10004; | &#10004; | &#63;
Arch Linux (latest) | Arch | &#10004; | &#63; | &#10007; | N/A | &#10007; | &#10004; | &#10004; | &#10004; | &#63;
All other linux | Other | &#63; | &#63; | &#63; | &#10007; | &#10007; | &#63; | &#63; | &#63; | &#63;


## x86 Architecture

TBD

## Supported functionalities accross different distribution channels

At this section we try to depict how we distribute our various functionalities, across the different distribution channels available.
There are various limitations and problems we try to attend as we evolve and grow and through this documentation we want to provide some clarity as to 
what is available and in what way. Of course we strive to deliver our full solution through all channels, but that may not be feasible at once.

**Legend**:

- **Auto-detect**: Depends on the programs package dependencies. If the required dependencies are covered during compile time, capability is enabled
- **YES**: This flag imply that the functionality is available for that distribution channel                
- **NO**: That means the availability is a work in progress for netdata, and not available at the moment for that distribution channel. We constantly work to provide everything to our users on all possible ways
- **Runtime-enabled**: This means that the given module or functionality is available and only requires configuration after install to enable it                

### Core functionality

#### Core
This is the base netdata capability, that includes basic monitoring, embedded web server, and so on.

| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| YES | YES | YES | YES | YES | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `install-required-packages.sh netdata`

#### DB Engine
This is the brand new database engine capability of netdata. It is a mandatory facility required by netdata. Given it's special needs and dependencies though, it remains an optional facility so that users can enjoy netdata even when they cannot cover the dependencies or the H/W requirements.

| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | YES | YES | YES | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `openssl`, `libuv1`, `lz4`, `Judy`

#### Encryption Support (HTTPS)
This is netdata's SSL capability that incorporates encryption on the web server and the APIs between master and slaves. Also a mandatory facility for netdata, but remains optional for users who are limited or not interested in tight security

| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | YES | YES | YES | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `openssl`


### Libraries/optimizations

#### JSON-C Support
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `json-c`

#### Link time optimizations
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | Auto-detect | Auto-detect | Auto-detect |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: No package dependency, depends on GCC version

### Collectors

#### FREEIPMI
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | No | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `freeipmi-dev (or -devel)`


#### NFACCT
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `libmnl-dev`, `libnetfilter_acct-dev`


#### Xenstat

| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | NO | NO |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `xen-dom0-libs-devel`, `yajl-dev`


#### CUPS
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `cups-devel`


#### FPING
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled |

- **Flags/instructions to enable**: ${INSTALL_PATH}/netdata/plugins.d/fping.plugin install
- **What packages required for auto-detect?**: None - only fping installed to start it up


#### IOPING
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled |

- **Flags/instructions to enable**: ${INSTALL_PATH}/netdata/plugins.d/ioping.plugin install
- **What packages required for auto-detect?**: None - only ioping installed to start it up


#### PERF
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled | Runtime-enabled |

- **Flags/instructions to enable**: Inside netdata.conf, section `[Plugins]`, set `"perf = yes"`
- **What packages required for auto-detect?**: None


### Backends


#### Prometheus remote write
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | YES | YES |

- **Flags/instructions to enable**: None
- **What packages required for auto-detect?**: `snappy-devel`, `protobuf`, `protobuf-compiler`

#### AWS Kinesis
| make/make install    | netdata-installer.sh | kickstart.sh | kickstart-static64.sh | Docker image | RPM packaging | DEB packaging |
| :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: | :-----------------: |
| Auto-detect | Auto-detect | Auto-detect | Auto-detect | NO | NO | NO |

- **Flags/instructions to enable**: [Instructions for AWS Kinesis](https://docs.netdata.cloud/backends/aws_kinesis)
- **What packages required for auto-detect?**: `AWS SDK for C++`, `libcurl`, `libssl`, `libcrypto`
