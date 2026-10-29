# Static Build Infrastructure

Netdata uses a custom build harness to produce standalone static builds that require nothing from the host except
for a handful of universally available commands for the installation itself.

The build process produces self extracting archive files, one using gzip compression, and one using zstd compression,
in the `artifacts` subdirectory of this directory.

Building requires the following tools:

- GNU Make 4.4 or newer
- Python 3.11 or newer
- A recent version of either Docker or Podman
- A recent version of Git
- A recent version of GNU Coreutils
- A recent version of GNU Binutils
- GNU wget
- Gzip for creation of the gzip compressed archive
- Zstd for creation of the zstd compressed archive
- When cross-compiling, a Linux kernel that supports binfmt\_misc

To create static builds for the current system’s CPU architecture, simply run `make -j $(nproc)` in this
directory. If cross-compiling, use `BUILDARCH` to specify the target CPU architecture (it should match the output
of `uname -m` for the target system), for example `make -j $(nproc) BUILDARCH=aarch64` to build for 64-bit ARM
regardless of the underlying hardware. In either case, the generated archives will be put under the `archives`
directory.

Most of the components of the build other than Netdata are automatically cached under `cache` to speed up subsequent
rebuilds. You can explicitly drop this cache using the `clear-cache` target.
