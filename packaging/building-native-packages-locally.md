# Build native (DEB/RPM) packages for testing

This document provides instructions for developers who need to build native packages locally for testing.

## Requirements

To build native packages locally, you will need the following:

* A working Docker or Podman host.
* A local copy of the source tree you want to build from.

## Building the packages

In the root of the source tree from which you want to build, clean up any existing files left over from a previous build
and then run:

```bash
docker run -it --rm -e VERSION=0.1 -v $PWD:/netdata netdata/package-builders:<tag>
```

or

```bash
podman run -it --rm -e VERSION=0.1 -v $PWD:/netdata netdata/package-builders:<tag>
```

The `<tag>` should be the lowercase distribution name with no spaces, followed by the
release of that distribution. For example, `centos7` to build on CentOS 7, or `ubuntu20.04`
to build on Ubuntu 20.04. Note that we use Rocky Linux for builds on CentOS/RHEL 8 or newer. See
[netdata/package-builders](https://hub.docker.com/r/netdata/package-builders/tags) for all available tags.

The value passed in the `VERSION` environment variable can be any version number accepted by the type of package
being built. As a general rule, it needs to start with a digit, and must include a `.` somewhere.

Once it finishes, the built packages can be found under `artifacts/` in the source tree.

If an error is encountered and the build is being run interactively, it will drop to a shell to allow you to
inspect the state of the container and look at build logs.

### Detailed explanation

The environments used for building our packages are fully self-contained Docker images built from [Dockerfiles](https://github.com/netdata/helper-images/tree/master/package-builders)
These are published on Docker
Hub with the image name `netdata/package-builders`, and tagged using the name and version of the distribution
(with the tag corresponding to the suffix on the associated Dockerfile).

The build code expects the following requirements to be met:

* It expects the source tree it should build from to be located at `/netdata`, and expects that said source tree
  is clean (no artifacts left over from previous builds).
* It expects an environment variable named `VERSION` to be defined, and uses this to control what version number
  will be shown in the package metadata and filenames.

Internally, the source tree gets copied to a temporary location for the build process so that the source tree can
be mounted directly from the host without worrying about leaving a dirty tree behind, any templating or file
movements required for the build to work are done, the package build command is invoked with the correct arguments,
and then the resultant packages are copied to the `artifacts/` directory in the original source tree so they are
accessible after the container exits.

## Finding build logs after a failed build

Build logs and artifacts can be found in the build directory, whose location varies by distribution.

On DEB systems (Ubuntu and Debian), the build directory inside the container is located at `/usr/src/netdata`

On RPM systems except openSUSE, the build directory inside the container is located under `/root/rpmbuild/BUILD/`
and varies based on the package version number.

On openSUSE, the build directory inside the container is located under `/usr/src/packages/BUILD`and varies based
on the package version number.

## Building for other architectures

If you need to test a build for an architecture that does not match your host system, you can do so by setting up
QEMU user-mode emulation. This requires a Linux kernel with binfmt\_misc support (all modern distributions provide
this out of the box, but I’m not sure about WSL or Docker Desktop).

The quick and easy way to do this is to run the following:

```bash
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
```

or

```bash
podman run --rm --privileged multiarch/qemu-user-static --reset -p yes
```

This will set up the required QEMU user-mode emulation until you reboot. Note that if using Podman, you will need
to run this as root and not as a rootless container (the package builds work fine in a rootless container though,
even if doing cross-architecture builds).

Once you have that set up, the command to build the packages is the same as above, you just need to add a correct
`--platform` option to the `docker run` or `podman run` command. The current list of architectures we build for,
and the correct value for the `--platform` option is:

* 32-bit ARMv7: `linux/arm/v7`
* 64-bit ARMv8: `linux/arm64/v8`
* 32-bit x86: `linux/i386`
* 64-bit x86: `linux/amd64`
