import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with Docker

## Limitations running the Agent in Docker

We don’t officially support running our Docker images with the Docker CLI `--user` option or the Docker Compose
`user:` parameter. Such usage will usually still work, but some features will not be available when run this
way. Note that the Agent will drop privileges appropriately inside the container during startup, meaning that even
when run without these options, almost nothing in the container will actually run with an effective UID of 0.

Our POWER8+ Docker images don’t support our FreeIPMI collector. This is a technical limitation in FreeIPMI itself,
and unfortunately, not something we can realistically work around.

## Create a new Netdata Agent container

You can create a new Agent container using either `docker run` or `docker-compose`. After using any method, you can
visit the Agent dashboard `http://NODE:19999`.

The Netdata container requires different privileges and mounts to provide functionality similar to that provided by
Netdata installed on the host. Below you can find a list of Netdata components that need these privileges and mounts,
along with their descriptions.

<details open>
<summary>Privileges</summary>

|       Component       |          Privileges           | Description                                                                                                              |
|:---------------------:|:-----------------------------:|--------------------------------------------------------------------------------------------------------------------------|
|    cgroups.plugin     |   host PID mode, SYS_ADMIN    | Container network interfaces monitoring. Map virtual interfaces in the system namespace to interfaces inside containers. |
|      proc.plugin      |       host network mode       | Host system networking stack monitoring.                                                                                 |
|      go.d.plugin      |       host network mode       | Monitoring applications running on the host and inside containers.                                                       |
|    local-listeners    | host network mode, SYS_PTRACE | Discovering local services/applications. Map open (listening) ports to running services/applications.                    |
| network-viewer.plugin | host network mode, SYS_ADMIN  | Discovering all current network sockets and building a network-map.                                                      |

</details>

<details open>
<summary>Mounts</summary>

|       Component        |           Mounts           | Description                                                                                                                                      |
|:----------------------:|:--------------------------:|--------------------------------------------------------------------------------------------------------------------------------------------------|
|        netdata         |      /etc/os-release       | Host info detection.                                                                                                                             |
|    diskspace.plugin    |             /              | Host mount points monitoring.                                                                                                                    |
|     cgroups.plugin     | /sys, /var/run/docker.sock | Docker containers monitoring and name resolution.                                                                                                |
|      go.d.plugin       |    /var/run/docker.sock    | Docker Engine and containers monitoring. See [docker](https://github.com/netdata/go.d.plugin/tree/master/modules/docker#readme) collector.       |
|      go.d.plugin       |          /var/log          | Web servers logs tailing. See [weblog](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog#readme) collector.                      |
|      apps.plugin       |  /etc/passwd, /etc/group   | Monitoring of host system resource usage by each user and user group.                                                                            |
|      proc.plugin       |           /proc            | Host system monitoring (CPU, memory, network interfaces, disks, etc.).                                                                           |
| systemd-journal.plugin |          /var/log          | Viewing, exploring and analyzing systemd journal logs.                                                                                           |
| systemd-journal.plugin |         /run/dbus          | Systemd-list-units function: information about all systemd units, including their active state, description, whether they are enabled, and more. |
|      go.d.plugin       |         /run/dbus          | [go.d/systemdunits](https://github.com/netdata/go.d.plugin/tree/master/modules/systemdunits#readme)                                              |

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
  -v /:/host/root:ro,rslave \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /etc/localtime:/etc/localtime:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  -v /var/log:/host/var/log:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /run/dbus:/run/dbus:ro \
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
      - /:/host/root:ro,rslave
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /etc/localtime:/etc/localtime:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/log:/host/var/log:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /run/dbus:/run/dbus:ro

volumes:
  netdataconfig:
  netdatalib:
  netdatacache:
```

</TabItem>
</Tabs>

> :bookmark_tabs: Note
>
> If you plan to connect the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

### With NVIDIA GPUs monitoring

Monitoring NVIDIA GPUs requires:

- Using official [NVIDIA driver](https://www.nvidia.com/Download/index.aspx).
- Installing [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html).
- Allowing the Netdata container to access GPU resources.

<Tabs>
<TabItem value="docker_run" label="docker run">

<h3> Using the <code>docker run</code> command </h3>

Add `--gpus 'all,capabilities=utility'` to your `docker run`.

</TabItem>
<TabItem value="docker compose" label="docker-compose">

<h3> Using the <code>docker-compose</code> command</h3>

Add the following to the netdata service.

```yaml
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

</TabItem>
</Tabs>

### With host-editable configuration

Use a [bind mount](https://docs.docker.com/storage/bind-mounts/) for `/etc/netdata` rather than a volume.

This example assumes that you’ve created `netdataconfig/` in your home directory.

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
  -v /:/host/root:ro,rslave \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /etc/localtime:/etc/localtime:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  -v /var/log:/host/var/log:ro \
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
      - /:/host/root:ro,rslave
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /etc/localtime:/etc/localtime:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/log:/host/var/log:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  netdatalib:
  netdatacache:
```

</TabItem>
</Tabs>

### With SSL/TLS enabled HTTP Proxy

For a permanent installation on a public server, you
should [secure the Netdata instance](/docs/netdata-agent/securing-netdata-agents.md). This
section contains an example of how to install Netdata with an SSL reverse proxy and basic authentication.

You can use the following `docker-compose.yml` and Caddyfile files to run Netdata with Docker. Replace the domains and
email address for [Let's Encrypt](https://letsencrypt.org/) before starting.

#### Caddyfile

This file needs to be placed in `/opt` with name `Caddyfile`. Here you customize your domain, and you need to provide
your email address to obtain a Let's Encrypt certificate. Certificate renewal will happen automatically and will be
executed internally by the caddy server.

```caddyfile
netdata.example.org {
  reverse_proxy host.docker.internal:19999
  tls admin@example.org
}
```

#### docker-compose.yml

After setting Caddyfile run this with `docker-compose up -d` to have a fully functioning Netdata setup behind an HTTP reverse
proxy.

Make sure Netdata bind to docker0 interface if you've custom `web.bind to` setting in `netdata.conf`.

```yaml
version: '3'
services:
  caddy:
    image: caddy:2
    extra_hosts:
      - "host.docker.internal:host-gateway" # To access netdata running with "network_mode: host".
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
      - /:/host/root:ro,rslave
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /etc/localtime:/etc/localtime:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/log:/host/var/log:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
volumes:
  caddy_data:
  caddy_config:
  netdataconfig:
  netdatalib:
  netdatacache:
```

#### Restrict access with basic auth

You can restrict access by
following the [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines
to Caddyfile.

### With Docker socket proxy

> **Note**: Using Netdata with a Docker socket proxy might have some features not working as expected. It hasn't been fully tested by the Netdata team.

Deploy a Docker socket proxy that accepts and filters out requests using something like
[HAProxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md) or
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
      - /:/host/root:ro,rslave
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /etc/localtime:/etc/localtime:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/log:/host/var/log:ro
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

> Note: This deployment method is supported by the community

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
      - /:/host/root:ro,rslave
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /etc/localtime:/etc/localtime:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
      - /var/log:/host/var/log:ro
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

### Rootless mode

Netdata can be run successfully in a non-root environment, such as [rootless Docker](https://docs.docker.com/engine/security/rootless/).

However, it should be noted that Netdata's data collection capabilities are considerably restricted in rootless Docker
due to its inherent limitations. While Netdata can function in a rootless environment, it can’t access certain
resources that require elevated privileges. The following components don’t work:

- container network interfaces monitoring (cgroup-network helper)
- disk I/O and file descriptors of applications and processes (apps.plugin)
- debugfs.plugin
- freeipmi.plugin
- perf.plugin
- slabinfo.plugin
- systemd-journal.plugin

This method creates a [volume](https://docs.docker.com/storage/volumes/) for Netdata's configuration files
_within the container_ at `/etc/netdata`.
See the [configure section](#configure-agent-containers) for details. If you want to access the configuration files from
your _host_ machine, see [host-editable configuration](#with-host-editable-configuration).

<Tabs>
<TabItem value="docker_run" label="docker run">

<h3> Using the <code>docker run</code> command </h3>

Run the following command in your terminal to start a new container.

```bash
docker run -d --name=netdata \
  --hostname=$(hostname) \
  -p 19999:19999 \
  -v netdataconfig:/etc/netdata \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /etc/localtime:/etc/localtime:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  -v /run/user/$UID/docker.sock:/var/run/docker.sock:ro \
  --restart unless-stopped \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

</TabItem>

</Tabs>

> :bookmark_tabs: Note
>
> If you plan to connect the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

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
the host in the container. With `docker run`, this can be done by adding `--volume /etc/hostname:/host/etc/hostname:ro` to
the options. If you’re using Docker Compose, you can add an entry to the container's `volumes` section
reading `- /etc/hostname:/host/etc/hostname:ro`.

## Adding extra packages at runtime

By default, the official Netdata container images don’t include a number of optional runtime dependencies. You
can add these dependencies, or any other APT packages, at runtime by listing them in the environment variable
`NETDATA_EXTRA_DEB_PACKAGES`.

Commonly useful packages include:

- `apcupsd`: For monitoring APC UPS devices.
- `lm-sensors`: For monitoring hardware sensors.
- `netcat-openbsd`: For IRC alert support.

## Health Checks

Our Docker image provides integrated support for health checks through the standard Docker interfaces.

You can control how the health checks run by using the environment variable `NETDATA_HEALTHCHECK_TARGET` as follows:

- If left unset, the health check will attempt to access the `/api/v1/info` endpoint of the Agent.
- If set to the exact value 'cli', the health check script will use `netdatacli ping` to determine if the Agent is
  running correctly or not. This is sufficient to ensure that Netdata didn’t hang during startup, but doesn’t provide
  a rigorous verification that the daemon is collecting data or is otherwise usable.
- If set to anything else, the health check will treat the value as a URL to check for a 200 status code on. In most
  cases, this should start with `http://localhost:19999/` to check the Agent running in the container.

In most cases, the default behavior of checking the `/api/v1/info` endpoint will be enough. If you’re using a
configuration which disables the web server or restricts access to certain APIs, you will need to use a non-default
configuration for health checks to work.

## Publish a test image to your own repository

At Netdata, we provide multiple ways of testing your Docker images using your own repositories.
You may either use the command line tools available or take advantage of our GitHub Actions infrastructure.
