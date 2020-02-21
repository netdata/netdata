# External build-system

This wraps the build-system in Docker so that the host system and the target system are
decoupled. This allows:
* Cross-compilation (e.g. linux development from MacOS)
* Cross-distro (e.g. using CentOS user-land while developing on Debian)
* Multi-host scenarios (e.g. master/slave configurations)
* Bleeding-edge sceneraios (e.g. using the ACLK (**currently for internal-use only**))

The advantage of these scenarios is that they allow **reproducible** builds and testing
for developers. This is the first iteration of the build-system to allow the team to use
it and get used to it.

For configurations that involve building and running the agent alone, we still use
`docker-compose` for consistency with more complex configurations. The more complex
configurations allow the agent to be run in conjunction with parts of the cloud
infrastructure (these parts of the code are not public), or with external brokers
(such as VerneMQ for MQTT), or with other external tools (such as TSDB to allow the agent to
export metrics). Note: no external TSDB scenarios are available in the first iteration,
they will be added in subsequent iterations.

This differs from the packaging dockerfiles as it designed to be used for local development.
The main difference is that these files are designed to support incremental compilation in
the following way:

1. The initial build should be performed using `bin/clean-install.sh` to create a docker
   image with the agent built from the source tree and installed into standard system paths
   using `netdata-installer.sh`. In addition to the steps performed by the standard packaging
   builds a manifest is created to allow subsequent builds to be made incrementally using
   `make` inside the container. Any libraries that are required for 'bleeding-edge' development
   are added on top of the standard install.
2. When the `bin/make-install.sh` script is used the docker container will be updated with
   a sanitized version of the current build-tree. The manifest will be used to line up the
   state of the incoming docker cache with `make`'s view of the file-system according to the
   manifest. This means the `make install` inside the container will only rebuild changes
   since the last time the disk image was created.

The exact improvement on the compile-cycle depends on the speed of the network connection
to pull the netdata dependencies, but should shrink the time considerably. For example,
on a macbook pro the initial install takes about 1min + network delay [Note: there is
something bad happening with the standard installer at the end of the container build as
it tries to kill the running agent - this is very slow and bad] and the incremental
step only takes 15s. On a debian host with a fast network this reduces 1m30 -> 13s.

## Examples

1. Simple cross-compilation / cross-distro builds.

```bash
build_external/bin/clean-install.sh arch current
docker run -it --rm arch_current_dev
echo >>daemon/main.c     # Simulate edit by touching file
build_external/bin/make-install.sh arch current
docker run -it --rm arch_current_dev
```

Currently there is no detection of when the installer needs to be rerun (really this is
when the `autoreconf` / `configure` step must be rerun). Netdata was not written with
multi-stage builds in mind and we need to work out how to do this in the future. For now
it is up to you to know when you need to rerun the clean build step.

```bash
build_external/bin/clean-install.sh arch current
build_external/bin/clean-install.sh ubuntu 19.10
docker run -it --rm arch_current_dev
echo >>daemon/main.c     # Simulate edit by touching file
build_external/bin/make-install.sh arch current
docker run -it --rm arch_current_dev
echo >>daemon/daemon.c     # Simulate second edit step
build_external/bin/make-install.sh arch current   # Observe a single file is rebuilt
build_external/bin/make-install.sh arch current   # Observe both files are rebuilt
```

The state of the build in the two containers is independent.

2. Single agent config in docker-compose

This functions the same as the previous example but is wrapped in docker-compose to
allow injection into more complex test configurations.

```bash
Distro=debian Version=10 docker-compose -f projects/only-agent/docker-compose.yml up
```

Note: it is possible to run multiple copies of the agent using the `--scale` option for
`docker-compose up`.

```bash
Distro=debian Version=10 docker-compose -f projects/only-agent/docker-compose.yml up --scale agent=3
```

3. A simple master-slave scenario

```bash
# Need to call clean-install on the configs used in the master/slave containers
docker-compose -f master-slaves/docker-compose.yml up --scale agent_slave1=2
```

Note: this is not production ready yet, but it is left in so that we can see how it behaves
and improve it. Currently it produces the following problems:
  * Only the base-configuration in the compose without scaling works.
  * The containers are hard-coded in the compose.
  * There is no way to separate the agent configurations, so running multiple agent slaves
    wth the same GUID kills the master which exits with a fatal condition.

4. The ACLK

This is for internal use only as it requires access to a private repo. Clone the vernemq-docker
repo and follow the instructions within to build an image called `vernemq`.

```bash
build_external/bin/clean-install.sh arch current  # Only needed first time
docker-compose -f build_external/projects/aclk-testing/vernemq-compose.yml -f build_external/projects/aclk-testing/agent-compose.yml up --build
```

Notes:
* We are currently limited to arch because of restrictions on libwebsockets
* There is not yet a good way to configure the target agent container from the docker-compose command line.
* Several other containers should be in this compose (a paho client, tshark etc).

