# Netdata static binary build

We publish pre-built static builds of Netdata for Linux systems. Currently, these are published for 64-bit x86, ARMv7,
AArch64, and POWER8+ hardware. These static builds are able to operate in a mostly self-contained manner and only
require a POSIX compliant shell and a supported init system. These static builds install under `/opt/netdata`. If
you are on a platform which we provide static builds for but do not provide native packages for, a static build
will be used by default for installation.

If you want to enforce the usage of a static build and have the installer return a failure if one is not available,
you can do so by adding `--static-only` to the options you pass to the installer.

## Requirements

- Container runtime tool (Docker or Podman)

## Building a static binary package

Before you begin, make sure that your repo and the repo's submodules are clean from any previous builds and up to date.
Otherwise, [perform a cleanup](/packaging/installer/methods/manual.md#perform-a-cleanup-in-your-netdata-repo)


To build the static binary 64-bit distribution package, into the root folder on the netdata repo, run:

```bash
./packaging/makeself/build-static.sh x86_64
```

The program will:

1.  setup a new docker container with Alpine Linux
2.  install the required alpine packages (the build environment, needed libraries, etc)
3.  download and compile third party apps that are packaged with Netdata (`bash`, `curl`, etc)
4.  compile Netdata

Once finished, a file named `netdata-vX.X.X-gGITHASH-x86_64-DATE-TIME.run` will be created in the current directory. This is the Netdata binary package that can be run to install Netdata on any other computer.

You can build static binaries for other architectures such as `armv7l`, `aarch64`, and `ppc64le`.

## Building binaries with debug info

To build Netdata binaries with debugging / tracing information in them, use:

```bash
cd /path/to/netdata.git
./packaging/makeself/build-static.sh x86_64 debug
```

These binaries are not optimized (they are a bit slower), they have certain features disables (like log flood protection), other features enables (like `debug flags`) and are not stripped (the binary files are bigger, since they now include source code tracing information).

## Debugging Netdata binaries

Once you have installed a binary package with debugging info, you will need to install `valgrind` and run this command to start Netdata:

```bash
PATH="/opt/netdata/bin:${PATH}" valgrind --undef-value-errors=no /opt/netdata/bin/srv/netdata -D
```

The above command, will run Netdata under `valgrind`. While Netdata runs under `valgrind` it will be 10x slower and use a lot more memory.

If Netdata crashes, `valgrind` will print a stack trace of the issue. Open a github issue to let us know.

To stop Netdata while it runs under `valgrind`, press Control-C on the console.

> If you omit the parameter `--undef-value-errors=no` to valgrind, you will get hundreds of errors about conditional jumps that depend on uninitialized values. This is normal. Valgrind has heuristics to prevent it from printing such errors for system libraries, but for the static Netdata binary, all the required libraries are built into Netdata. So, valgrind cannot apply its heuristics and prints them.
> 
