<!--
title: "Manually build Netdata from source"
description: "Package maintainers and power users may be interested in manually building Netdata from source without using any of our installation scripts."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/source.md
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
-   GNU autogen
-   GNU libtool
-   pkg-config
-   GCC or Xcode (Clang is known to have issues in certain configurations, see [Using Clang](#using-clang))
-   A version of `make` compatible with GNU automake
-   Git (we use git in the build system to generate version info, don't need a full install, just a working `git show` command)

Additionally, the following build time features require additional dependencies:

-   TLS support for the web GUI:
    -   OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
-   dbengine metric storage:
    -   liblz4 r129 or newer
    -   OpenSSL 1.0 or newer _or_ LibreSSL 3.0.0 or newer.
-   Netdata Cloud support:
    -   A working internet connection
    -   A recent version of CMake
    -   OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
    -   JSON-C (may be provided by the user as shown below, or by the system)
    -   protobuf (Google Protocol Buffers) and protoc compiler
-   Machine Learning support:
    -   A working C++ compiler
-   eBPF monitoring support on Linux:
    -   libelf 0.0.6 or newer (for legacy eBPF) _or_ libelf 0.5 or newer (for CO-RE)

## Preparing the source tree

Certain features in Netdata require custom versions of specific libraries,
which the the build system will link statically into Netdata. These
libraries and their header files must be copied into specific locations
in the source tree to be used.

### If using a git checkout
#### Submodules

Our git repository uses submodules for certain copoments. To obtain a complete build when using a git checkout,
make sure you either clone the repository with the `--recursive` option, or that you run `git submodule update
--init --recursivea` in your local copy of the repository before attempting to build Netdata.

#### Handling version numbers with git checkouts

When building from a git checkout, our build systems requires the git history at least as far back as the most
recent tag to ensure that the correct version number is used. In most cases, this means that a shallow clone
created with `--depth=1` will only work if you are building a stable version and cloned the associated tag directly.

### Netdata cloud support
#### JSON-C

Netdata requires the use of JSON-C for JSON parsing when using Netdata
Cloud. Netdata is able to use a system-provided copy of JSON-C, but
some systems may not provide it. If your system does not provide JSON-C,
you can do the following to prepare a copy for the build system:

1.  Verify the tag that Netdata expects to be used by checking the contents
    of `packaging/jsonc.version` in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/json-c/json-c and downloading
        and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Prepare the JSON-C sources by running `cmake -DBUILD_SHARED_LIBS=OFF .`
    in the JSON-C source directory.
4.  Build JSON-C by running `make` in the JSON-C source directory.
5.  In the Netdata source directory, create a directory called
    `externaldeps/jsonc`.
6.  Copy `libjson-c.a` from the JSON-C source directory to
    `externaldeps/jsonc/libjson-c.a` in the Netdata source tree.
7.  Copy all of the header files (`*.h`) from the JSON-C source directory
    to `externaldeps/jsonc/json-c` in the Netdata source tree.

### eBPF support
#### libbpf

Netdata requires a custom version of libbpf for eBPF support on Linux, which will be statically linked by the
build system. You can do the following to prepare a copy for the build system:

1.  Verify the tag that Netdata expects to be used by checking the contents of `packaging/libbpf_current.version`
    in your Netdata sources.
2.  Obtain the sources for that version by either:
    -   Navigating to https://github.com/netdata/libbpf and downloading and unpacking the source code archive for
        that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Prepare the libbpf sources by running `make -p src/root src/build` in the libbpf source tree.
4.  Build libbpf by running `BUILD_STATIC_ONLY=y OBJDIR=build DESTDIR=.. make install` in the `src/` subdirectory
    of the libbpf source tree.
5.  In the Netdata source directory, create a directory called `externaldeps/libbpf`
6.  Copy `libbpf.a` from the libbpf source directory to `externaldeps/libbpf/libbpf.a` in the Netdata source tree.
    -   On 32-bit hosts, `libbpf.a` is located at `usr/lib/libbpf.a` in the libbpf source tree.
    -   On 64-bit hosts, `libbpf.a` is located at `usr/lib64/libbpf.a` in the libbpf source tree.
7.  Copy the `usr/include` directory from the libbpf source tree to `externaldeps/libbpf/include` in the Netdata
    source tree.
8.  Copy the `include/uapi` directory from the libbpf source tree to `externaldeps/libbpf/include/uapi` in the
    Netdata source tree.

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
-   `--with-webdir`: Specify a path relative to the prefix in which to install the web UI files.
-   `--disable-cloud`: Disables all Netdata Cloud functionality for this build.
-   `--disable-ml`: Disable ML support in Netdata (results in a much faster and smaller build).

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

### Go collectors

A number of the collectors for Netdata are written in Go instead of C,
and are developed in a separate repository from the mian Netdata code.
An installation without these collectors is still usable, but will be
unable to collect metrics for a number of network services the system
may be providing. You can either install a pre-built copy of these
collectors, or build them locally.

#### Installing the pre-built Go collectors

We provide pre-built binaries of the Go collectors for all the platforms
we officially support. To use one of these:

1.  Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/go.d.version` in your Netdata sources.
2.  Go to https://github.com/netdata/go.d.plugin/releases, select the
    required release, and download the `go.d.plugin-*.tar.gz` file
    for your system type and CPu architecture and the `config.tar.gz`
    configuration file archive.
3.  Extract the `go.d.plugin-*.tar.gz` archive into a temporary
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
    -   Navigating to https://github.com/netdata/go.d.plugin and downloading
        and unpacking the source code archive for that release.
    -   Cloning the repository with `git` and checking out the required tag.
3.  Run `make` in the go.d.plugin source tree.
4.  Copy `bin/godplugin` to `/usr/libexec/netdata/plugins.d` or th
    equivalent location for your build of Netdata and rename it to
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
    file for the libc variant your system uses (either rmusl or glibc).
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
