<!--
title: "Deploy the Agent"
sidebar_label: "Deploy the Agent"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/deploy-the-agent.md"
learn_status: "Published"
sidebar_position: "20"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
learn_docs_purpose: "Step by step instructions to deploy an Agent."
-->

import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Admonition from '@theme/Admonition';

This document will guide you through the installation of the Netdata solution to your preferred platform and claim it in the Cloud.

### Linux/UNIX based installation via kickstart (reccomended)

This section will guide you through installation using the automatic one-line installation script named `kickstart.sh`.

:::info
The kickstart script works on all Linux distributions and, by default, automatic nightly updates are enabled.
:::

#### Prerequisites

- Connection to the internet
- A Linux/UNIX based node
- Either `wget` or `curl` installed on the node

#### Steps

<!-- TODO: kickstart must validate first the integrity and then run the actual kickstart script -->

1. Verify script integrity

    To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
    run the following:

    ```bash
    [ "<checksum-will-be-added-in-documentation-processing>" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
    ```

    If the script is valid, this command will return `OK, VALID`.

2. Install Netdata by running one of the following options:

    <Tabs>
    <TabItem value="wget" label=<code>wget</code>>

    <OneLineInstallWget/>

    </TabItem>
    <TabItem value="curl" label=<code>curl</code>>

    <OneLineInstallCurl/>

    </TabItem>
    </Tabs>

    If you want to see all the optional parameters to further alter your installation, check
    the [kickstart script reference](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md)
    .

 

#### Expected result

The script should exit with a success message.  
To ensure that your installation is working, open up your web browser of choice and navigate to `http://NODE:19999`,
replacing `NODE` with the IP address or hostname of your node.  
If you're interacting with the node locally, and you are unsure of its IP address, try `http://localhost:19999` first.

If the installation was successful, you will be led to the Agent's local dashboard. Enjoy!


#### Related topics

1. [Kickstart script reference](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md)

### Docker runtime installation

Running the Netdata Agent in a container works best for an internal network or to quickly analyze a host. Docker helps
you get set up quickly, and doesn't install anything permanent on the system, which makes uninstalling the Agent easy.

This method creates a [bind mount](https://docs.docker.com/storage/bind-mounts/) for Netdata's configuration files
_within the container_ at `/etc/netdata`. See the [configuration section](#configure-agent-containers) for details. If
you want to access the configuration files from your _host_ machine, see [host-editable
configuration](#host-editable-configuration).

See our full list of Docker images at [Docker Hub](https://hub.docker.com/r/netdata/netdata).

<!--
#### Limitations running the Agent in Docker

For monitoring the whole host, running the Agent in a container can limit its capabilities. Some data, like the host OS
performance or status, is not accessible or not as detailed in a container as when running the Agent directly on the
host.

A way around this is to provide special mounts to the Docker container so that the Agent can get visibility on host OS
information like `/sys` and `/proc` folders or even `/etc/group` and shadow files. -->

<!-- MAKE THIS INTO NEW TASKS
Also, we now ship Docker images using an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint)
directive, not a COMMAND directive. Please adapt your execution scripts accordingly. You can find more information about
ENTRYPOINT vs COMMAND in the [Docker
documentation](https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact).

Our POWER8+ Docker images do not support our FreeIPMI collector. This is a technical limitation in FreeIPMI itself,
and unfortunately not something we can realistically work around.
-->

#### Prerequisites

- `docker` installed on the node.

#### Steps

1. Use the `docker run` command, along with the following options, to start a new container.

    ```bash
    docker run -d --name=netdata \
      -p 19999:19999 \
      -v netdataconfig:/etc/netdata \
      -v netdatalib:/var/lib/netdata \
      -v netdatacache:/var/cache/netdata \
      -v /etc/passwd:/host/etc/passwd:ro \
      -v /etc/group:/host/etc/group:ro \
      -v /proc:/host/proc:ro \
      -v /sys:/host/sys:ro \
      -v /etc/os-release:/host/etc/os-release:ro \
      --restart unless-stopped \
      --cap-add SYS_PTRACE \
      --security-opt apparmor=unconfined \
      netdata/netdata
    ```

### Docker-compose installation

Running the Netdata Agent in a container works best for an internal network or to quickly analyze a host. Docker helps
you get set up quickly, and doesn't install anything permanent on the system, which makes uninstalling the Agent easy.

This method creates a [bind mount](https://docs.docker.com/storage/bind-mounts/) for Netdata's configuration files
_within the container_ at `/etc/netdata`. See the [configuration section](#configure-agent-containers) for details. If
you want to access the configuration files from your _host_ machine, see [host-editable
configuration](#host-editable-configuration).

See our full list of Docker images at [Docker Hub](https://hub.docker.com/r/netdata/netdata).

<!--
#### Limitations running the Agent in Docker

For monitoring the whole host, running the Agent in a container can limit its capabilities. Some data, like the host OS
performance or status, is not accessible or not as detailed in a container as when running the Agent directly on the
host.

A way around this is to provide special mounts to the Docker container so that the Agent can get visibility on host OS
information like `/sys` and `/proc` folders or even `/etc/group` and shadow files. -->

<!-- MAKE THIS INTO NEW TASKS
Also, we now ship Docker images using an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint)
directive, not a COMMAND directive. Please adapt your execution scripts accordingly. You can find more information about
ENTRYPOINT vs COMMAND in the [Docker
documentation](https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact).

Our POWER8+ Docker images do not support our FreeIPMI collector. This is a technical limitation in FreeIPMI itself,
and unfortunately not something we can realistically work around.
-->

#### Prerequisites

- `docker`
- `docker-compose` 

installed on the node.


#### Steps

1. Copy the following code and paste into a new file called `docker-compose.yml`, then run
`docker-compose up -d` in the same directory as the `docker-compose.yml` file to start the container.

    ```yaml
    version: '3'
    services:
      netdata:
        image: netdata/netdata
        container_name: netdata
        hostname: example.com # set to fqdn of host
        ports:
          - 19999:19999
        restart: unless-stopped
        cap_add:
          - SYS_PTRACE
        security_opt:
          - apparmor:unconfined
        volumes:
          - netdataconfig:/etc/netdata
          - netdatalib:/var/lib/netdata
          - netdatacache:/var/cache/netdata
          - /etc/passwd:/host/etc/passwd:ro
          - /etc/group:/host/etc/group:ro
          - /proc:/host/proc:ro
          - /sys:/host/sys:ro
          - /etc/os-release:/host/etc/os-release:ro
    volumes:
      netdataconfig:
      netdatalib:
      netdatacache:
    ```

<!-- MAKE THIS A CONCEPT
#### Docker tags

The official `netdata/netdata` Docker image provides the following named tags:

- `stable`: The `stable` tag will always point to the most recently published stable build.
- `edge`: The `edge` tag will always point ot the most recently published nightly build. In most cases, this is
  updated daily at around 01:00 UTC.
- `latest`: The `latest` tag will always point to the most recently published build, whether it’s a stable build
  or a nightly build. This is what Docker will use by default if you do not specify a tag.

Additionally, for each stable release, three tags are pushed, one with the full version of the release (for example,
`v1.30.0`), one with just the major and minor version (for example, `v1.30`), and one with just the major version
(for example, `v1`). The tags for the minor versions and major versions are updated whenever a release is published
that would match that tag (for example, if `v1.30.1` were to be published, the `v1.30` tag would be updated to
point to that instead of `v1.30.0`).
-->

<!-- MAKE THIS A TASK, GROUP IT SOMEHOW
#### Health Checks

Our Docker image provides integrated support for health checks through the standard Docker interfaces.

You can control how the health checks run by using the environment variable `NETDATA_HEALTHCHECK_TARGET` as follows:

- If left unset, the health check will attempt to access the
  `/api/v1/info` endpoint of the agent.
- If set to the exact value 'cli', the health check
  script will use `netdatacli ping` to determine if the agent is running
  correctly or not. This is sufficient to ensure that Netdata did not
  hang during startup, but does not provide a rigorous verification
  that the daemon is collecting data or is otherwise usable.
- If set to anything else, the health check will treat the value as a
  URL to check for a 200 status code on. In most cases, this should
  start with `http://localhost:19999/` to check the agent running in
  the container.

In most cases, the default behavior of checking the `/api/v1/info`
endpoint will be sufficient. If you are using a configuration which
disables the web server or restricts access to certain APIs, you will
need to use a non-default configuration for health checks to work.
-->

### Docker-compose with SSL/TLS enabled HTTP Proxy installation

For a permanent installation on a public server, you should [secure the Netdata
instance](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/secure-agent-deployments.md). This section contains an example of
how to install Netdata with an SSL reverse proxy and basic authentication.

You can use the following `docker-compose.yml` and Caddyfile files to run Netdata with Docker. Replace the domains and
email address for [Let's Encrypt](https://letsencrypt.org/) before starting.

#### Steps

1. Caddyfile

    This file needs to be placed in `/opt` with name `Caddyfile`. Here you customize your domain, and you need to provide
    your email address to obtain a Let's Encrypt certificate. Certificate renewal will happen automatically and will be
    executed internally by the caddy server.

    ```caddyfile
    netdata.example.org {
      reverse_proxy netdata:19999
      tls admin@example.org
    }
    ```

2. Create the docker-compose.yml file

    After setting Caddyfile, run this with `docker-compose up -d` to have a fully functioning Netdata setup behind an HTTP
    revers proxy.

    ```yaml
    version: '3'
    volumes:
      caddy_data:
      caddy_config:
    services:
      caddy:
        image: caddy:2
        ports:
          - "80:80"
          - "443:443"
        volumes:
          - /opt/Caddyfile:/etc/caddy/Caddyfile
          - caddy_data:/data
          - caddy_config:/config
      netdata:
        restart: always
        hostname: netdata.example.org
        image: netdata/netdata
        cap_add:
          - SYS_PTRACE
        security_opt:
          - apparmor:unconfined
        volumes:
          - netdatalib:/var/lib/netdata
          - netdatacache:/var/cache/netdata
          - /etc/passwd:/host/etc/passwd:ro
          - /etc/group:/host/etc/group:ro
          - /proc:/host/proc:ro
          - /sys:/host/sys:ro
          - /var/run/docker.sock:/var/run/docker.sock:ro
    volumes:
      netdatalib:
      netdatacache:
    ```

<!-- THIS IS ANOTHER TASK
#### Restrict access with basic auth

You can restrict access by
following [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines
to Caddyfile.
-->

<!-- THIS IS ANOTHER TASK
#### Configure Agent containers

If you started an Agent container using one of the [recommended methods](#steps), and you want to edit Netdata's
configuration, you must first use `docker exec` to attach to the container. Replace `netdata`
with the name of your container.

```bash
docker exec -it netdata bash
cd /etc/netdata
./edit-config netdata.conf
```


You need to restart the Agent to apply changes. Exit the container if you haven't already, then use the `docker` command
to restart the container: `docker restart netdata`.
-->

<!-- THIS IS ANOTHER TASK
##### Host-editable configuration

:::caution
The `edit-config` script doesn't work when executed on the host system.
:::

If you want to make your container's configuration directory accessible from the host system, you need to use a
[volume](https://docs.docker.com/storage/bind-mounts/) rather than a bind mount. The following commands create a
temporary `netdata_tmp` container, which is used to populate a `netdataconfig` directory, which is then mounted inside
the container at `/etc/netdata`.

```bash
mkdir netdataconfig
docker run -d --name netdata_tmp netdata/netdata
docker cp netdata_tmp:/etc/netdata netdataconfig/
docker rm -f netdata_tmp
```

###### `docker run`

Use the `docker run` command, along with the following options, to start a new container. Note the
changed `-v $(pwd)/netdataconfig/netdata:/etc/netdata:ro \` line from the recommended example above.

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v $(pwd)/netdataconfig/netdata:/etc/netdata:ro \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  --restart unless-stopped \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

###### `docker compose`

Copy the following code and paste into a new file called `docker-compose.yml`, then run
`docker-compose up -d` in the same directory as the `docker-compose.yml` file to start the container. Note the
changed `./netdataconfig/netdata:/etc/netdata:ro` line from the recommended example above.

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    hostname: example.com # set to fqdn of host
    ports:
      - 19999:19999
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
    security_opt:
      - apparmor:unconfined
    volumes:
      - ./netdataconfig/netdata:/etc/netdata:ro
      - netdatalib:/var/lib/netdata
      - netdatacache:/var/cache/netdata
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
volumes:
  netdatalib:
  netdatacache:
```

-->


<!-- THIS IS ANOTHER TASK and also for the kickstart
##### Change the default hostname

You can change the hostname of a Docker container, and thus the name that appears in the local dashboard and in Netdata
Cloud, when creating a new container. If you want to change the hostname of a Netdata container _after_ you started it,
you can safely stop and remove it. Your configuration and metrics data reside in persistent volumes and are reattached
to the recreated container.

If you use `docker-run`, use the `--hostname` option with `docker run`.

```bash
docker run -d --name=netdata \
  --hostname=my_docker_netdata
```

If you use `docker-compose`, add a `hostname:` key/value pair into your `docker-compose.yml` file, then create the
container again using `docker-compose up -d`.

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    hostname: my_docker_compose_netdata
    ...
```

If you don't want to destroy and recreate your container, you can edit the Agent's `netdata.conf` file directly. See the
above section on [configuring Agent containers](#configure-agent-containers) to find the appropriate method based on
how you created the container.

-->

<!-- THIS IS ANOTHER TASK
##### Add or remove other volumes

Some volumes are optional depending on how you use Netdata:

- If you don't want to use the apps.plugin functionality, you can remove the mounts of `/etc/passwd` and `/etc/group`
  (they are used to get proper user and group names for the monitored host) to get slightly better security.
- Most modern linux distros supply `/etc/os-release` although some older distros only supply `/etc/lsb-release`. If
  this is the case you can change the line above that mounts the file inside the container to
  `-v /etc/lsb-release:/host/etc/lsb-release:ro`.
- If your host is virtualized then Netdata cannot detect it from inside the container and will output the wrong
  metadata (e.g. on `/api/v1/info` queries). You can fix this by setting a variable that overrides the detection
  using, e.g. `--env VIRTUALIZATION=$(systemd-detect-virt -v)`. If you are using a `docker-compose.yml` then add:

```yaml
    environment:
      - VIRTUALIZATION=${VIRTUALIZATION}
```

This allows the information to be passed into `docker-compose` using:

```bash
VIRTUALIZATION=$(systemd-detect-virt -v) docker-compose up
```

###### _Files inside systemd volumes_

If a volume is used by systemd service, some files can be removed during
[reinitialization](https://github.com/netdata/netdata/issues/9916). To avoid this, you need to add
`RuntimeDirectoryPreserve=yes` to the service file.

##### Docker container names resolution

There are a few options for resolving container names within Netdata. Some methods of doing so will allow root access to
your machine from within the container. Please read the following carefully.

###### _Docker socket proxy (safest option)_

Deploy a Docker socket proxy that accepts and filters out requests using something like
HAProxy so that it restricts connections to read-only access to the CONTAINERS
endpoint.

The reason it's safer to expose the socket to the proxy is because Netdata has a TCP port exposed outside the Docker
network. Access to the proxy container is limited to only within the network.

Below is [an example repository (and image)](https://github.com/Tecnativa/docker-socket-proxy) that provides a proxy to
the socket.

You run the Docker Socket Proxy in its own Docker Compose file and leave it on a private network that you can add to
other services that require access.

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    # ... rest of your config ...
    ports:
      - 19999:19999
    environment:
      - DOCKER_HOST=proxy:2375
  proxy:
    image: tecnativa/docker-socket-proxy
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - CONTAINERS=1
```

**Note:** Replace `2375` with the port of your proxy.

###### _Giving group access to the Docker socket (less safe)_

> You should seriously consider the necessity of activating this option, as it grants to the `netdata`
> user access to the privileged socket connection of docker service and therefore your whole machine.
> If you want to have your container names resolved by Netdata, make the `netdata` user be part of the group that owns
> the
> socket.
```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    # ... rest of your config ...
    volumes:
      # ... other volumes ...
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - PGID=[GROUP NUMBER]
```

To achieve that just add environment variable `PGID=[GROUP NUMBER]` to the Netdata container, where `[GROUP NUMBER]` is
practically the group id of the group assigned to the docker socket, on your host.

This group number can be found by running the following (if socket group ownership is docker):

```bash
grep docker /etc/group | cut -d ':' -f 3
```

###### _Running as root (unsafe)_

> You should seriously consider the necessity of activating this option, as it grants to the `netdata` user access to
> the privileged socket connection of docker service, and therefore your whole machine.
```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    # ... rest of your config ...
    volumes:
      # ... other volumes ...
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - DOCKER_USR=root
```

-->

<!--THIS IS ANOTHER TASK

##### Docker container network interfaces monitoring

Netdata can map a virtual interface in the system namespace to an interface inside a Docker container
when using network [bridge](https://docs.docker.com/network/bridge/) driver. To do this, the Netdata container needs
additional privileges:

- the host PID mode. This turns on sharing between container and the host operating system the PID
  address space (needed to get list of PIDs from `cgroup.procs` file).

- `SYS_ADMIN` capability (needed to execute `setns()`).

**docker run**:

```bash
docker run -d --name=netdata \
  ...
  --pid=host \
  --cap-add SYS_ADMIN \
  ...
  netdata/netdata
```

**docker compose**:

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    pid: host
    cap_add:
      - SYS_ADMIN
    ...
```

-->

<!-- ANOTHER TASK
##### Pass command line options to Netdata

Since we use an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint) directive, you can provide
[Netdata daemon command line options](https://github.com/netdata/netdata/blob/master/daemon/README.md#command-line-options) such as the IP address Netdata will be
running on, using the [command instruction](https://docs.docker.com/engine/reference/builder/#cmd).
-->


### Kubernetes cluster installation via Helm

This document details how to install Netdata on an existing Kubernetes (k8s) cluster. By following these directions, you
will use Netdata's [Helm chart](https://github.com/netdata/helmchart) to create a Kubernetes monitoring deployment on
your cluster.

The Helm chart installs one `parent` pod for storing metrics and managing alarm notifications, plus an additional
`child` pod for every node in the cluster, responsible for collecting metrics from the node, Kubernetes control planes,
pods/containers, and [supported application-specific
metrics](https://github.com/netdata/helmchart#service-discovery-and-supported-services).

#### Prerequisites

To deploy Kubernetes monitoring with Netdata, you need:

- A working cluster running Kubernetes v1.9 or newer.
- The [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool, within [one minor version
  difference](https://kubernetes.io/docs/tasks/tools/install-kubectl/#before-you-begin) of your cluster, on an
  administrative system.
- The [Helm package manager](https://helm.sh/) v3.0.0 or newer on the same administrative system.

#### Steps

We recommend you install the Helm chart using our Helm repository. In the `helm install` command, replace `netdata` with
the release name of your choice.

1. ```bash
    helm repo add netdata https://netdata.github.io/helmchart/
    helm install netdata netdata/netdata
    ```

2. Run `kubectl get services` and `kubectl get pods` to confirm that your cluster now runs a `netdata` service, one
   parent pod, and multiple child pods.

#### Expected result

You've now installed Netdata on your Kubernetes cluster. Next, it's time to enable the powerful Kubernetes
dashboards available in Netdata Cloud! To do so check out our Task
on [claiming an Agent to the Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md).
