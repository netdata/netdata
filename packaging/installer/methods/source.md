# Manually build Netdata from source

These instructions are for advanced users and distribution package
maintainers. Unless this describes you, you almost certainly want
to follow [our guide for manually installing Netdata from a git
checkout](/packaging/installer/methods/manual.md) instead.

## Required dependencies

At a bare minimum, Netdata requires the following libraries and tools
to build and run successfully:

- libuuid
- libuv version 1.0 or newer
- zlib
- CMake 3.13 or newer
- GCC or Xcode (Clang is known to have issues in certain configurations, see [Using Clang](#using-clang))
- Ninja or Make (Ninja is preferred as it results in significantly faster builds)
- Git (we use git in the build system to generate version info, you don't need a full install, just a working `git show` command)

The following additional dependencies are also needed, but will be prepared automatically by CMake if they are not available on the build system.

- libyaml
- JSON-C

Additionally, the following build time features require additional dependencies:

- TLS support for the web GUI:
  - OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
- dbengine metric storage:
  - liblz4 r129 or newer
  - OpenSSL 1.0 or newer (LibreSSL _amy_ work, but is largely untested).
- Netdata Cloud support:
  - A working internet connection
  - OpenSSL 1.0.2 or newer _or_ LibreSSL 3.0.0 or newer.
  - protobuf (Google Protocol Buffers) and protoc compiler. If protobuf is not available on the system,
        CMake can be instructed to fetch and build a usable version for Netdata.
- Netdata Go collectors:
  - Go 1.21 or newer

## Preparing the source tree

Netdata uses Git submodules for some of it’s components, which must be fetched prior to building Netdata. If you
are using a source tarball published by the Netdata project, then these are included. If you are using a checkout
of the Git repository, you may need to explicitly fetch and update the submodules using `git submodule update
--init --recursive`.

### Netdata Cloud

## Building Netdata

Once the source tree has been prepared, Netdata is ready to be configured
and built. Netdata uses CMake for configuration, and strongly prefers
the use of an external build directory. To configure and build Netdata:

1. Run `cmake -S . -B build -G Ninja` in the source tree. `build` can be replaced with whatever path you want for the build directory. If you wish to use Make instead of Ninja for the build, remove the `-G Ninja` from the command.
2. Run `cmake --build build`, where `build` is the build directory. CMake’s `--parallel` option can be used to control the number of build jobs that are used.
3. Run `cmake --install build`, where `build` is the build directory.

### Configure options

Netdata’s CMake build infrastructure intentionally does very little auto-detection, and requires most components
to be explicitly enabled or disabled. A full list of available configuration options for a given version of Netdata,
with help descriptions, can be seen by running `cmake -LH` in the source tree.

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

### eBPF collector

On Linux systems, Netdata has support for using the kernel's eBPF
interface to monitor performance-related VFS, network, and process events,
allowing for insights into process lifetimes and file access
patterns. Using this functionality requires additional code managed in
a separate repository from the core Netdata Agent. You can either install
a pre-built copy of the required code, or build it locally.

#### Installing the pre-built eBPF code

We provide pre-built copies of the eBPF code for 64-bit x86 systems
using glibc or musl. To use one of these:

1. Verify the release version that Netdata expects to be used by checking
    the contents of `packaging/ebpf.version` in your Netdata sources.
2. Go to <https://github.com/netdata/kernel-collector/releases>, select the
    required release, and download the `netdata-kernel-collector-*.tar.xz`
    file for the libc variant your system uses (either rmusl or glibc).
3. Extract the contents of the archive to a temporary location, and then
    copy all of the `.o` and `.so.*` files and the contents of the `library/`
    directory to `/usr/libexec/netdata/plugins.d` or the equivalent location
    for your build of Netdata.

#### Building the eBPF code locally

Alternatively, you may wish to build the eBPF code locally yourself. For
instructions, please consult [the README file for our kernel-collector
repository](https://github.com/netdata/kernel-collector/#readme),
which outlines both the required dependencies, as well as multiple
options for building the code.
