<!--
title: "Manually build Netdata from source"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/source.md
description: "Package maintainers and power users may be interested in manually building Netdata from source without using any of our installation scripts."
-->

# Manually build Netdata from source

These instructions are for advanced users and distribution package
maintainers. Unless this describes you, you almost certainly want
to follow [our guide for manually installing Netdata from a git
checkout](/packaging/installer/methods/manual.md) instead.

## Required dependencies

At a bare minimum, Netdata requires the following libraries and tools
to build and run successfully:

-   libuuid
-   libuv version 1.0 or newer
-   zlib
-   GNU autoconf
-   GNU automake
-   GCC or Xcode (Clang is known to have issues in certain configurations, see [Using Clang](#using-clang))
-   A version of `make` compatible with GNU automake
-   Git (we use git in the build system to generate version info, don't need a full install, just a working `git show` command)

Additionally, the following build time features require additional dependencies:

-   TLS support for the web GUI:
    -   OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
-   dbengine metric storage:
    -   liblz4 r129 or newer
    -   OpenSSL 1.0 or newer (LibreSSL _amy_ work, but is largely untested).
    -   [libJudy](http://judy.sourceforge.net/)
-   Netdata Cloud support:
    -   A working internet connection
    -   A recent version of CMake
    -   OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
    -   JSON-C (may be provided by the user as shown below, or by the system)

## Preparing the source tree

Certain features in Netdata require custom versions of specific libraries,
which the the build system will link statically into Netdata. These
libraries and their header files must be copied into specific locations
in the source tree to be used.

### Netdata cloud

Netdata Cloud functionality requires custom builds of libmosquitto and
libwebsockets.

#### libmosquitto

Netdata maintains a custom fork of libmosquitto at
https://github.com/netdata/mosquitto with patches to allow for proper
integration with libwebsockets, which is needed for correct operation of
Netdata Cloud functionality. To prepare this library for the build system:

1.  Verify the tag that Netdata expects to be used by checking the contents
    of `packaging/mosquitto.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/netdata/mosquitto/releases and
        donwloading and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  If building on a platfom other than Linux, prepare the mosquitto
    sources by running `cmake -D WITH_STATIC_LIBRARIES:boolean=YES .` in
    the mosquitto source directory.
4.  Build mosquitto by running `make -C lib` in the mosquitto source directory.
5.  In the Netdata source directory, create a directory called `externaldeps/mosquitto`.
6.  Copy `lib/mosquitto.h` from the mosquitto source directory to
    `externaldeps/mosquitto/mosquitto.h` in the Netdata source tree.
7.  Copy `lib/libmosquitto.a` from the mosquitto source directory to
    `externaldeps/mosquitto/libmosquitto.a` in the Netdata source tree. If
    building on a platform other than Linux, the file that needs to be
    copied will instead be named `lib/libmosquitto_static.a`, but it
    still needs to be copied to `externaldeps/mosquitto/libmosquitto.a`.

#### libwebsockets

Netdata uses the standard upstream version of libwebsockets located at
https://github.com/warmcat/libwebsockets, but requires a build with SOCKS5
support, which is not enabled by most pre-built versions. Currently,
we do not support using a system copy of libwebsockets. To prepare this
library for the build system:

1.  Verify the tag that Netdata expects to be used by checking the contents
    of `packaging/libwebsockets.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/warmcat/libwebsockets/releases and
        donwloading and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Prepare the libweboskcets sources by running `cmake -D
    LWS_WITH_SOCKS5:bool=ON .` in the libwebsockets source directory.
4.  Build libwebsockets by running `make` in the libwebsockets source
    directory.
5.  In the Netdata source directory, create a directory called
    `externaldeps/libwebsockets`.
6.  Copy `lib/libwebsockets.a` from the libwebsockets source directory to
    `externaldeps/libwebsockets/libwebsockets.a` in the Netdata source tree.
7.  Copy the entire contents of `lib/include` from the libwebsockets source
    directory to `externaldeps/libwebsockets/include` in the Netdata source tree.

#### JSON-C

Netdata requires the use of JSON-C for JSON parsing when using Netdata
Cloud. Netdata is able to use a system-provided copy of JSON-C, but
some systems may not provide it. If your system does not provide JSON-C,
you can do the following to prepare a copy for the build system:

1.  Verify the tag that Netdata expects to be used by checking the contents
    of `packaging/jsonc.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/json-c/json-c and donwloading
        and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Prepare the JSON-C sources by running `cmake -DBUILD_SHARED_LIBS=OFF .`
    in the JSON-C source directory.
4.  Build JSON-C by running `make` in the JSON-C source directory.
5.  In the Netdata source directory, create a directory called
    `externaldeps/jsonc`.
6.  Copy `libjson-c.a` fro the JSON-C source directory to
    `externaldeps/jsonc/libjson-c.a` in the Netdata source tree.
7.  Copy all of the header files (`*.h`) from the JSON-C source directory
    to `externaldeps/jsonc/json-c` in the Netdata source tree.

## Building Netdata

Once the source tree has been prepared, Netdata is ready to be configured
and built. Netdata currently uses GNU autotools as it's primary build
system. To build Netdata this way:

1.  Run `autoreconf -ivf` in the Netdata source tree.
2.  Run `./configure` in the Netdata source tree.
3.  Run `make` in the Netdata source tree.

### Configure options

Netdata provides a number of build time configure options. This section
lists some of the ones you are most likely to need:

-   `--prefix`: Specify the prefix under which Netdata will be installed.
-   `--with-webdir`: Specify a path relative to the prefix in which to
    install the web UI files.
-   `--disable-cloud`: Disables all Netdata Cloud functionality for
    this build.

### Using Clang

Netdata is primarily developed using GCC, but in most cases we also
build just fine using Clang. Under some build configurations of Clang
itself, you may see build failures with the linker reporting errors
about `nonrepresentable section on output`. We currently do not have a
conclusive fix for this issue (the obvious fix leads to other issues which
we haven't been able to fix yet), and unfortunately the only workaround
is to use a different build of Clang or to use GCC.

### Linking errors relating to OpenSSL

Netdata's build system currently does not reliably support building
on systems which have multiple ABI incompatible versions of OpenSSL
installed. In such situations, you may encounter linking errors due to
Netdata trying to build against headers for one version but link to a
different version.

## Additional components

A full featured install of Netdata requires some additional components
which must be built and installed separately from the main Netdata
agent. All of these should be handled _after_ installing Netdata itself.

### React dashboard

The above build steps include a deprecated web UI for Netdata that lacks
support for Netdata Cloud. To get a fully featured dashboard, you must
install our new React dashboard.

#### Installing the pre-built React dashboard

We provide pre-built archives of the React dashboard for each release
(these are also used during our normal install process). To use one
of these:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/dashboard.version` in your Netdata sources.
2.  Go to https://github.com/netdata/dashboard/releases and download the
    `dashboard.tar.gz` file for the required release.
3.  Unpack the downloaded archive to a temporary directory.
4.  Copy the contents of the `build` directory from the extracted
    archive to `/usr/share/netdata/web` or the equivalent location for
    your build of Netdata. This _will_ overwrite some files in the target
    location.

#### Building the React dashboard locally

Alternatively, you may wish to build the React dashboard locally. Doing
so requires a recent version of Node.JS with a working install of
NPM. Once you have the required tools, do the following:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/dashboard.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/netdata/dashboard and donwloading
        and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Run `npm install` in the dashboard source tree.
4.  Run `npm run build` in the dashboard source tree.
5.  Copy the contents of the `build` directory just like step 4 of
    installing the pre-built React dashboard.

### Go collectors

A number of the collectors for Netdata are written in Go instead of C,
and are developed in a separate repository from the mian Netdata code.
An installation without these collectors is still usable, but will be
unable to collect metrics for a number of network services the system
may be providing. You can either install a pre-built copy of these
eollectors, or build them locally.

#### Installing the pre-built Go collectors

We provide pre-built binaries of the Go collectors for all the platforms
we officially support. To use one of these:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/go.d.version` in your Netdata sources.
2.  Go to https://github.com/netdata/go.d.plugin/releases, select the
    required release, and download the `go.d.plugin-*.tar.gz` file
    for your system type and CPu architecture and the `config.tar.gz`
    configuration file archive.
3.  Extract the `go.d.plugin-*.tar.gz` archive into a temprary
    location, and then copy the single file in the archive to
    `/usr/libexec/netdata/plugins.d` or the equivalent location for your
    build of Netdata and rename it to `go.d.plugin`.
4.  Extract the `config.tar.gz` archive to a temporarylocation and then
    copy the contents of the archive to `/etc/netdata` or the equivalent
    location for your build of Netdata.

#### Building the Go collectors locally

Alternatively, you may wish to build the Go collectors locally
yourself. Doing so requires a working installation of Golang 1.13 or
newer. Once you have the required tools, do the following:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/go.d.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/netdata/go.d.plugin and donwloading
        and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Run `make` in the go.d.plugin source tree.
4.  Copy `bin/godplugin` to `/usr/libexec/netdata/plugins.d` or th
    eequivalent location for your build of Netdata and rename it to
    `go.d.plugin`.
5.  Copy the contents of the `config` directory to `/etc/netdata` or the
    equivalent location for your build of Netdata.

### eBPF collector

On Linux systems, Netdata has support for using the kernel's eBPF
interface to monitor performance-related VFS, network, and process events,
allowing for insights into process lifetimes and file access
patterns. Using this functionality requires additional code managed in
a separate repository from the core Netdata agent. You can either install
a pre-built copy of the required code, or build it locally.

#### Installing the pre-built eBPF code

We provide pre-built copies of the eBPF code for 64-bit x86 systems
using glibc or musl. To use one of these:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/ebpf.version` in your Netdata sources.
2.  Go to https://github.com/netdata/kernel-collector/releases, select the
    required release, and download the `netdata-kernel-collector-*.tar.xz`
    file for the libc variant your system uses (eithe rmusl or glibc).
3.  Extract the contents of the archive to a temporary location, and then
    copy all of the `.o` and `.so.*` files and the contents of the `library/`
    directory to `/usr/libexec/netdata/plugins.d` or the equivalent location
    for your build of Netdata.

#### Building the eBPF code locally

Alternatively, you may wish to build the eBPF code locally yourself. For
instructions, please consult [the README file for our kernel-collector
repository](https://github.com/netdata/kernel-collector/blob/master/README.md),
which outlines both the required dependencies, as well as multiple
options for building the code.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fsource.md&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
