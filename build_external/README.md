# External build-system

This allows the agent to be built and tested on various linux distros regardless of the
host system. In particular this allows MacOS to be used as a build host via Docker For
Mac. For configurations that involve building and running the agent alone, we still use
`docker-compose` for consistency with more complex configurations. The more complex
configurations allow the agent to be run in conjunction with parts of the cloud
infrastructure (these parts of the code are not public), or with external brokers
(such as VerneMQ for MQTT), or with other external tools (such as TSDB to allow the agent to
export metrics).

This differs from the packaging dockerfiles as it designed to be used for local development.
The main difference is that these files are designed to support incremental compilation in
the following way:

1. The initial build should use of the `clean_build` dockerfiles to create a docker image
   with the agent built from the source tree and installed into standard system paths
   using `netdata-installer.sh`.
2. When one of the `make_install` dockerfiles is used it will start on an image with the
   last built version of the agent, the docker-cache will be aligned with changes in the
   source repo (to prevent rebuilding intermediate images or re-running install steps),
   the `make install` inside the container will only rebuild the changed files.

The exact improvement on the compile-cycle depends on the speed of the network connection
to pull the netdata dependencies, but should shrink the time considerably. For example,
on a macbook pro the initial install takes about 1min + network delay and the incremental
step only takes 15s. On a debian host with a fast network this reduces 1m30 -> 13s.

# NOT UPDATED YET

Each of the Dockerfiles is a known configuration (linux distro and version). To use the
build-system it needs to be configured with an appropriate source. This can be done in
two ways:

1. Build a local copy of the source (pulled from the HEAD of the main repo).
2. Link to an existing working directory containing the source.

The idea is that option (1) can be used in automated testing sequences to ensure that
the local source is independent, and that option (2) can be used during active development
allowing the persistence of changes to the source tree across build / test steps.

In both cases the source tree is not copied into the agent container - it is bind-mounted
into the container file-system to prevent multiple copies of the tree existing during
development (i.e. the version of the code in the container is *always* the copy on the
host that is being developed).

## Scripts

| Script                  | Purpose |
| ----------------------- | ------- |
| `bin/fetch.sh` | Create (overwrite) directory with latest source |
| `bin/link.sh`| Create (overwrite) directory with link to source |
| `bin/preinst.sh` | Setup a config ready for installation of Netdata |
| `bin/clean_install.sh`| Run the netdata-installer |
| `bin/make_install.sh`| Run a `make; make install` inside the container |
| `bin/run.sh`| Execute the Netdata agent inside the container |

## Instantiation in Docker

Each configuration results in two images in Docker and a container that is reused:

* `distro_version_preinst` is the disk image in a state ready to install Netdata.
* `distro_version_dev` is the disk image after the installer is executed.
* `distro_version_dev` is the container (re-)used for building / executing Netdata.

## Example usage

```
bin/link.sh ~/netdata/local.repo
bin/preinst.sh ubuntu 19.10
bin/clean_install.sh ubuntu 19.10
bin/run.sh ubuntu 19.10
... edit cycle ...
bin/make_install.sh ubuntu 19.10
bin/run.sh ubuntu 19.10
```

## Notes

CentOS is not properly supported yet. Because of the missing `Judy-devel` package in 
CentOS 8 the plan is to pull in Judy separately and compile it locally. The `Dockerfile`
is setup to get this far but I cannot test it further as trying to run a CentOS container
hangs on Docker For Mac.

