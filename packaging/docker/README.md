<!--
title: "Install Netdata with Docker"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/packaging/docker/README.md"
sidebar_label: "Docker"
learn_status: "Published"
learn_rel_path: "Installation/Installation methods"
sidebar_position: 40
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with Docker

## Create a new Netdata Agent container

You can create a new Agent container using either `docker run` or `docker-compose`. After using any method, you can
visit the Agent dashboard `http://NODE:19999`.

The Netdata container requires different privileges and mounts to provide functionality similar to that provided by
Netdata installed on the host. Below you can find a list of Netdata components that need these privileges and mounts,
along with their descriptions.

<details>
<summary>Privileges</summary>

|    Component    |          Privileges           | Description                                                                                                              | 
|:---------------:|:-----------------------------:|--------------------------------------------------------------------------------------------------------------------------|
| cgroups.plugin  |   host PID mode, SYS_ADMIN    | Container network interfaces monitoring. Map virtual interfaces in the system namespace to interfaces inside containers. |
|   proc.plugin   |       host network mode       | Host system networking stack monitoring.                                                                                 |
|   go.d.plugin   |       host network mode       | Monitoring applications running on the host and inside containers.                                                       |
| local-listeners | host network mode, SYS_PTRACE | Discovering local services/applications. Map open (listening) ports to running services/applications.                    |

</details>

<details>
<summary>Mounts</summary>

|   Component    |           Mounts           | Description                                                                                                                         | 
|:--------------:|:--------------------------:|-------------------------------------------------------------------------------------------------------------------------------------|
|    netdata     |      /etc/os-release       | Host info detection.                                                                                                                |
| cgroups.plugin | /sys, /var/run/docker.sock | Docker containers monitoring and name resolution.                                                                                   |
|  go.d.plugin   |    /var/run/docker.sock    | Docker Engine and containers monitoring. See [docker](https://github.com/netdata/go.d.plugin/tree/master/modules/docker) collector. |
|  apps.plugin   |  /etc/passwd, /etc/group   | Monitoring of host system resource usage by each user and user group.                                                               |
|  proc.plugin   |           /proc            | Host system monitoring (CPU, memory, network interfaces, disks, etc.).                                                              |

</details>

### Recommended way

Both methods create a [volume](https://docs.docker.com/storage/volumes/) for Netdata's configuration files
_within the container_ at `/etc/netdata`.
See the [configure section](#configure-agent-containers) for details. If you want to access the configuration files from
your _host_ machine, see [host-editable configuration](#with-host-editable-configuration).

<Tabs>
<TabItem value="docker_run" label="docker run">

<h3> Using the <code>docker run</code> command </h3>

Run the following command in your terminal to start a new container.

```bash
docker run -d --name=netdata \
  --pid=host \
  --network=host \
  -v netdataconfig:/etc/netdata \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  --restart unless-stopped \
  --cap-add SYS_PTRACE \
  --cap-add SYS_ADMIN \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

</TabItem>
<TabItem value="docker compose" label="docker-compose">

<h3> Using the <code>docker-compose</code> command</h3>

Create a file named `docker-compose.yml` in your project directory and paste the code below. From your project
directory, start Netdata by running `docker-compose up -d`.

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    pid: host
    network_mode: host
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
      - SYS_ADMIN
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
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  netdataconfig:
  netdatalib:
  netdatacache:
```

</TabItem>
</Tabs>

> :bookmark_tabs: Note
>
> If you plan to Claim the node to Netdata Cloud, you can find the command with the right parameters by clicking the "
> Add Nodes" button in your Space's "Nodes" view.

### With host-editable configuration

Use a [bind mount](https://docs.docker.com/storage/bind-mounts/) for `/etc/netdata` rather than a volume.

This example assumes that you have created `netdataconfig/` in your home directory.

```bash
mkdir netdataconfig
```

<Tabs>
<TabItem value="docker_run" label="docker run">

<h3> Using the <code>docker run</code> command </h3>

Run the following command in your terminal to start a new container.

```bash
docker run -d --name=netdata \
  --pid=host \
  --network=host \
  -v $(pwd)/netdataconfig/netdata:/etc/netdata \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  --restart unless-stopped \
  --cap-add SYS_PTRACE \
  --cap-add SYS_ADMIN \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

</TabItem>
<TabItem value="docker compose" label="docker-compose">

<h3> Using the <code>docker-compose</code> command</h3>

Create a file named `docker-compose.yml` in your project directory and paste the code below. From your project
directory, start Netdata by running `docker-compose up -d`.

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    pid: host
    network_mode: host
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
      - SYS_ADMIN
    security_opt:
      - apparmor:unconfined
    volumes:
      - ./netdataconfig/netdata:/etc/netdata
      - netdatalib:/var/lib/netdata
      - netdatacache:/var/cache/netdata
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  netdatalib:
  netdatacache:
```

</TabItem>
</Tabs>

### With SSL/TLS enabled HTTP Proxy

For a permanent installation on a public server, you
should [secure the Netdata instance](https://github.com/netdata/netdata/blob/master/docs/netdata-security.md). This
section contains an example of how to install Netdata with an SSL reverse proxy and basic authentication.

You can use the following `docker-compose.yml` and Caddyfile files to run Netdata with Docker. Replace the domains and
email address for [Let's Encrypt](https://letsencrypt.org/) before starting.

#### Caddyfile

This file needs to be placed in `/opt` with name `Caddyfile`. Here you customize your domain, and you need to provide
your email address to obtain a Let's Encrypt certificate. Certificate renewal will happen automatically and will be
executed internally by the caddy server.

```caddyfile
netdata.example.org {
  reverse_proxy netdata:19999
  tls admin@example.org
}
```

#### docker-compose.yml

After setting Caddyfile run this with `docker-compose up -d` to have a fully functioning Netdata setup behind an HTTP reverse
proxy.

```yaml
version: '3'
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
    image: netdata/netdata
    container_name: netdata
    hostname: example.com # set to fqdn of host
    restart: always
    pid: host
    cap_add:
      - SYS_PTRACE
      - SYS_ADMIN
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
      - /var/run/docker.sock:/var/run/docker.sock:ro
volumes:
  caddy_data:
  caddy_config:
  netdatalib:
  netdatacache:
```

#### Restrict access with basic auth

You can restrict access by
following the [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines
to Caddyfile.

### With Docker socket proxy

Deploy a Docker socket proxy that accepts and filters out requests using something like
[HAProxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md) or
[CetusGuard](https://github.com/hectorm/cetusguard) so that it restricts connections to read-only access to
the `/containers` endpoint.

The reason it's safer to expose the socket to the proxy is because Netdata has a TCP port exposed outside the Docker
network. Access to the proxy container is limited to only within the network.

#### HAProxy

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    pid: host
    network_mode: host
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
      - SYS_ADMIN
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
    environment:
      - DOCKER_HOST=localhost:2375
  proxy:
    network_mode: host
    image: tecnativa/docker-socket-proxy
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - CONTAINERS=1

volumes:
  netdataconfig:
  netdatalib:
  netdatacache:
```

**Note:** Replace `2375` with the port of your proxy.

#### CetusGuard

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    pid: host
    network_mode: host
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
      - SYS_ADMIN
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
    environment:
      - DOCKER_HOST=localhost:2375
  cetusguard:
    image: hectorm/cetusguard:v1
    network_mode: host
    read_only: true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      CETUSGUARD_BACKEND_ADDR: unix:///var/run/docker.sock
      CETUSGUARD_FRONTEND_ADDR: tcp://:2375
      CETUSGUARD_RULES: |
        ! Inspect a container
        GET %API_PREFIX_CONTAINERS%/%CONTAINER_ID_OR_NAME%/json

volumes:
  netdataconfig:
  netdatalib:
  netdatacache:
```

You can run the socket proxy in its own Docker Compose file and leave it on a private network that you can add to
other services that require access.

## Docker tags

See our full list of Docker images at [Docker Hub](https://hub.docker.com/r/netdata/netdata).

The official `netdata/netdata` Docker image provides the following named tags:

|   Tag    | Description                                                                                                                                             |
|:--------:|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `stable` | the most recently published stable build.                                                                                                               |
|  `edge`  | the most recently published nightly build. In most cases, this is updated daily at around 01:00 UTC.                                                    |
| `latest` | the most recently published build, whether it’s a stable build or a nightly build. This is what Docker will use by default if you do not specify a tag. |
| `vX.Y.Z` | the full version of the release (for example, `v1.40.0`).                                                                                               |
|  `vX.Y`  | the major and minor version (for example, `v1.40`).                                                                                                     |
|   `vX`   | just the major version (for example, `v1`).                                                                                                             |

The tags for minor and major versions are updated whenever a release that matches this tag is published (for example,
if `v1.40.1` were to be published, the `v1.40` tag would be updated to it instead of pointing to `v1.40.0`).

## Configure Agent containers

If you started an Agent container using one of the [recommended methods](#create-a-new-netdata-agent-container), and you
want to edit Netdata's configuration, you must first use `docker exec` to attach to the container. Replace `netdata`
with the name of your container.

```bash
docker exec -it netdata bash
cd /etc/netdata
./edit-config netdata.conf
```

You need to restart the Agent to apply changes. Exit the container if you haven't already, then use the `docker` command
to restart the container: `docker restart netdata`.

### Change the default hostname

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
```

If you don't want to destroy and recreate your container, you can edit the Agent's `netdata.conf` file directly. See the
above section on [configuring Agent containers](#configure-agent-containers) to find the appropriate method based on
how you created the container.

Alternatively, you can directly use the hostname from the node running the container by mounting `/etc/hostname` from
the host in the container. With `docker run`, this can be done by adding `--volume /etc/hostname:/etc/hostname:ro` to
the options. If you are using Docker Compose, you can add an entry to the container's `volumes` section
reading `- /etc/hostname:/etc/hostname:ro`.

## Adding extra packages at runtime

By default, the official Netdata container images do not include a number of optional runtime dependencies. You
can add these dependencies, or any other APK packages, at runtime by listing them in the environment variable
`NETDATA_EXTRA_APK_PACKAGES`.

Commonly useful packages include:

- `apcupsd`: For monitoring APC UPS devices.
- `libvirt-daemon`: For resolving cgroup names for libvirt domains.
- `lm-sensors`: For monitoring hardware sensors.
- `msmtp`: For email alert support.
- `netcat-openbsd`: For IRC alert support.

## Health Checks

Our Docker image provides integrated support for health checks through the standard Docker interfaces.

You can control how the health checks run by using the environment variable `NETDATA_HEALTHCHECK_TARGET` as follows:

- If left unset, the health check will attempt to access the `/api/v1/info` endpoint of the agent.
- If set to the exact value 'cli', the health check script will use `netdatacli ping` to determine if the agent is
  running correctly or not. This is sufficient to ensure that Netdata did not hang during startup, but does not provide
  a rigorous verification that the daemon is collecting data or is otherwise usable.
- If set to anything else, the health check will treat the value as a URL to check for a 200 status code on. In most
  cases, this should start with `http://localhost:19999/` to check the agent running in the container.

In most cases, the default behavior of checking the `/api/v1/info` endpoint will be sufficient. If you are using a
configuration which disables the web server or restricts access to certain APIs, you will need to use a non-default
configuration for health checks to work.

## Publish a test image to your own repository

At Netdata, we provide multiple ways of testing your Docker images using your own repositories.
You may either use the command line tools available or take advantage of our GitHub Actions infrastructure.
