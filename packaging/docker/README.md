import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with Docker

## Limitations running the Agent in Docker

We don’t officially support using Docker’s `--user` option or Docker Compose’s `user:` parameter with our images. While they may work, some features could be unavailable. The Agent drops privileges at startup, so most processes don’t run as UID 0 even without these options.

## Create a new Netdata Agent container

You can create a new Agent container with `docker run` or `docker-compose`, then access the dashboard at `http://NODE:19999`.

The Netdata container requires specific **privileges** and **mounts** to provide full monitoring capabilities equivalent to a direct host installation. Below is a list of required components and their purposes.

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
| systemd-units.plugin |         /run/dbus          | Systemd-list-units function: information about all systemd units, including their active state, description, whether they are enabled, and more. |
|      go.d.plugin       |         /run/dbus          | [go.d/systemdunits](https://github.com/netdata/go.d.plugin/tree/master/modules/systemdunits#readme)                                              |

</details>

### Recommended way

Both methods create a [volume](https://docs.docker.com/storage/volumes/) for Netdata's configuration files
_within the container_ at `/etc/netdata`.
See the [configure section](#configure-agent-containers) for details. If you want to access the configuration files from your _host_ machine, see [host-editable configuration](#with-host-editable-configuration).

:::info If you remove `pid: host`
If you choose **not** to use `pid: host`, you **must** add [`--init`](https://docs.docker.com/reference/cli/docker/container/run/#init) (or [`init: true`](https://docs.docker.com/reference/compose-file/services/#init) in Compose).

`--init` installs a minimal init system that reaps processes and ensures stable container operation.
:::

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

:::tip

- When using `netdata/netdata` without a tag, Docker pulls the latest image by default. To run the stable version, replace it with `netdata/netdata:stable`.
- If you plan to connect the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

:::

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

:::tip

- When using `netdata/netdata` without a tag, Docker pulls the latest image by default. To run the stable version, replace it with `netdata/netdata:stable`.
- If you plan to connect the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

:::

### With SSL/TLS enabled HTTP Proxy

Below is an example of installing Netdata with an **SSL reverse proxy** and **basic authentication** using Docker.

#### Caddyfile Setup

Place the following `Caddyfile` in `/opt`, customizing the domain and adding your email for **Let’s Encrypt**. The certificate will renew automatically via the Caddy server.

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

You can restrict access by following the [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines to Caddyfile.

### With Docker socket proxy

:::note

Using Netdata with a Docker socket proxy may cause some features to not work as expected. It hasn't been fully tested by the Netdata team.

:::

For better security, deploy a **Docker socket proxy** with a tool like [HAProxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md) or [CetusGuard](https://github.com/hectorm/cetusguard). This ensures the socket is **read-only** and restricted to the `/containers` endpoint.

Exposing the socket to a proxy is safer because Netdata’s TCP port is accessible outside the Docker network, while the proxy container remains isolated within it.

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

:::tip

- When using `netdata/netdata` without a tag, Docker pulls the latest image by default. To run the stable version, replace it with `netdata/netdata:stable`.
- Replace `2375` with the port of your proxy.

:::

#### CetusGuard

:::note

This deployment method is supported by the community

:::

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

:::tip

You can run the socket proxy in its own Docker Compose file and leave it on a private network that you can add to other services that require access.

:::

### Rootless mode

Netdata can be run successfully in a non-root environment, such as [rootless Docker](https://docs.docker.com/engine/security/rootless/).

Netdata can run in a rootless Docker environment, but its data collection is limited due to restricted access to resources requiring elevated privileges.
The following components won't work:

- container network interfaces monitoring (cgroup-network helper)
- disk I/O and file descriptors of applications and processes (apps.plugin)
- debugfs.plugin
- freeipmi.plugin
- perf.plugin
- slabinfo.plugin
- systemd-journal.plugin

This method creates a [volume](https://docs.docker.com/storage/volumes/) for Netdata's configuration files
_within the container_ at `/etc/netdata`.
See the [configure section](#configure-agent-containers) for details. If you want to access the configuration files from your _host_ machine, see [host-editable configuration](#with-host-editable-configuration).

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

:::tip

- When using `netdata/netdata` without a tag, Docker pulls the latest image by default. To run the stable version, replace it with `netdata/netdata:stable`.
- If you plan to connect the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

:::

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

Minor and major version tags update with each matching release. For example, if `v1.40.1` is published, the `v1.40` tag moves from `v1.40.0` to `v1.40.1`.

## Update your Netdata Docker container

Docker containers do not auto-update. To update, you pull a new image and recreate the container.

:::important

Persistent volumes (`netdataconfig`, `netdatalib`, `netdatacache`) preserve your configuration and metrics across container recreation. If you followed the recommended installation, these volumes are already set up.

:::

<Tabs>
<TabItem value="docker_run" label="docker run">

1. Pull the latest image:

   ```bash
   docker pull netdata/netdata:stable
   ```

2. Stop and remove the existing container:

   ```bash
   docker stop netdata && docker rm netdata
   ```

3. Recreate the container using the same `docker run` command you originally used (see [Create a new Netdata Agent container](#create-a-new-netdata-agent-container) above).

</TabItem>
<TabItem value="docker_compose" label="docker-compose">

```bash
docker-compose pull && docker-compose up -d
```

The `docker-compose.yml` file preserves all configuration options.

</TabItem>
</Tabs>

Check the running Agent version:

```bash
docker exec netdata netdata -W buildinfo
```

:::tip

Use the `stable` tag to pull only stable releases. The `latest` tag (the default) may include nightly builds. See [Docker tags](#docker-tags) for all available tags.

:::

:::note

If Netdata Cloud shows a **Critical update** notification, your Agent version is below the minimum required version for optimal Cloud functionality. Nightly builds may trigger this notification even when up to date. Switching to the `stable` tag resolves this.

:::

:::note

If you manage containers through a third-party platform (such as CasaOS, Portainer, or ZimaBoard), use that platform's update interface. The Netdata image must use our official image tags to receive updates.

:::

## Configure Agent Containers

If you started an Agent container using one of the [recommended methods](#create-a-new-netdata-agent-container) and need to edit its configuration, first attach to the container with `docker exec`, replacing `netdata` with your container’s name.

```bash
docker exec -it netdata bash
cd /etc/netdata
./edit-config netdata.conf
```

Restart the Agent to apply changes: exit the container if necessary, then run `docker restart netdata`.

### Change the default hostname

A container’s hostname appears in both the local dashboard and Netdata Cloud.

To change it after creation, stop and remove the container—it’s safe! Your configuration and metrics stay intact in persistent volumes and will reattach when you recreate the container.

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

If you prefer not to recreate the container, edit the Agent’s `netdata.conf` file. See [configuring Agent containers](#configure-agent-containers) for the right method based on how you created it.

Alternatively, use the **host’s hostname** by mounting `/etc/hostname` in the container:

- **With `docker run`**, add:
  ```sh
  --volume /etc/hostname:/host/etc/hostname:ro
  ```  
- **With Docker Compose**, add this to the `volumes` section:
  ```yaml
  - /etc/hostname:/host/etc/hostname:ro
  ```

## Environment Variables

The container's entrypoint reads these environment variables at startup. Set them with `docker run -e` or the `environment:` block in Docker Compose, the same way you set any container environment variable.

| Variable | Default | What it controls |
|:--|:--|:--|
| [`NETDATA_LISTENER_PORT`](#netdata_listener_port) | `19999` | TCP port the Agent's web server and API listen on. |
| [`NETDATA_HEALTHCHECK_TARGET`](#netdata_healthcheck_target) | `http://localhost:19999/api/v1/info` | What the Docker health check polls. |
| [`DOCKER_USR`](#docker_usr) | `netdata` | System user the Agent runs as. |
| [`DOCKER_HOST`](#docker_host-and-pgid) | Auto-detected | Location of the Docker daemon socket. |
| [`PGID`](#docker_host-and-pgid) | Auto-detected | GID of the host Docker group. |
| [`NETDATA_EXTRA_DEB_PACKAGES`](#netdata_extra_deb_packages) | *(empty)* | Extra Debian packages installed at container start. |
| [`NETDATA_EXTRA_APK_PACKAGES`](#netdata_extra_apk_packages-deprecated) | *(unset)* | **Deprecated.** No longer installs anything. |
| [`DISABLE_TELEMETRY`](#disable_telemetry-and-do_not_track) | `0` | Opts out of anonymous telemetry when set to a non-zero value. |
| [`DO_NOT_TRACK`](#disable_telemetry-and-do_not_track) | `0` | Same as `DISABLE_TELEMETRY`. |

### NETDATA_LISTENER_PORT

The TCP port the Agent's web server and API listen on. The default is `19999`.

When you change it, remember to publish the same port from the container. For example, to use port `30000`:

```bash
docker run -d --name=netdata \
  -e NETDATA_LISTENER_PORT=30000 \
  -p 30000:30000 \
  netdata/netdata
```

This variable also feeds the default [health check](#netdata_healthcheck_target) target, so the check follows the port automatically unless you override it.

### NETDATA_HEALTHCHECK_TARGET

Controls what the container's Docker `HEALTHCHECK` polls. By default the check requests the Agent's `/api/v1/info` endpoint over the [listener port](#netdata_listener_port): `http://localhost:19999/api/v1/info`.

Two modes:

- **A URL** (the default, or any value other than `cli`): the health check runs `curl` against that URL. The container is healthy if the request succeeds.
- **`cli`**: the health check runs `netdatacli ping`. This confirms the daemon process is alive, but does **not** verify the web server or that data collection is working.

Use `cli` when you have disabled the web server or restricted API access, so the default HTTP check would always report unhealthy:

```bash
docker run -d --name=netdata -e NETDATA_HEALTHCHECK_TARGET=cli netdata/netdata
```

You can also point it at a different endpoint or host when your setup requires it.

### DOCKER_USR

The system user the Agent runs as. The default is `netdata`.

The entrypoint uses this user when assigning supplemental group memberships (the Docker group, the Proxmox configuration group, and the NVIDIA device group), so collectors that need those groups keep working. In normal operation you do not need to change this value.

### DOCKER_HOST and PGID

`DOCKER_HOST` tells Netdata where to find the Docker daemon, and `PGID` is the group ID the Agent needs to read that socket. Both are **auto-detected** at container start.

#### Auto-detection

When the container starts as root, the entrypoint probes for a container runtime socket in this order:

1. **balenaEngine** — detected via `/var/run/balena.sock`. When found, the collector is pointed at `unix:///var/run/balena-engine.sock`, and `PGID` is set to that socket's group owner.
2. **Docker** — detected via `/var/run/docker.sock`. When found, the collector is pointed at `unix:///var/run/docker.sock`, and `PGID` is set to that socket's group owner.

The entrypoint then adds the `DOCKER_USR` user to a group with the detected `PGID` so the Agent can read the socket.

#### Overriding with a custom value

If neither socket is present, the entrypoint leaves `DOCKER_HOST` and `PGID` untouched, so any value you set is preserved. This is how **Docker socket proxy** setups work (see [With Docker socket proxy](#with-docker-socket-proxy) above): you do not mount `/var/run/docker.sock` into the container, and instead point Netdata at the proxy:

```yaml
environment:
  - DOCKER_HOST=localhost:2375
```

:::warning Detected sockets override your value
If `/var/run/docker.sock` (or `/var/run/balena.sock`) **is** mounted into the container, the entrypoint detects it and overrides any `DOCKER_HOST` and `PGID` you set. To use a custom `DOCKER_HOST` (for example, a socket proxy), do not mount the host socket into the container.
:::

### NETDATA_EXTRA_DEB_PACKAGES

A space-separated list of Debian packages installed with `apt-get` at container start, before the Agent starts. The default is empty (nothing is installed).

By default, Netdata's official container images exclude some optional runtime dependencies. This variable installs them at runtime. Commonly installed packages:

- `apcupsd` — monitors APC UPS devices.
- `lm-sensors` — monitors hardware sensors.
- `netcat-openbsd` — enables IRC alerts.

```bash
docker run -e NETDATA_EXTRA_DEB_PACKAGES="lm-sensors netcat-openbsd" netdata/netdata
```

Installation runs every time the container starts, so adding many packages increases startup time.

### NETDATA_EXTRA_APK_PACKAGES (deprecated)

**Deprecated.** The Netdata Docker image moved from Alpine to Debian as its base. This variable previously installed Alpine (`apk`) packages; it now installs nothing.

If it is set to a non-empty value, the container prints a warning at startup reminding you to use `NETDATA_EXTRA_DEB_PACKAGES` instead, then continues normally.

To silence the warning, unset the variable or set it to an empty string, and migrate your package list to [`NETDATA_EXTRA_DEB_PACKAGES`](#netdata_extra_deb_packages) using Debian package names.

### DISABLE_TELEMETRY and DO_NOT_TRACK

Set **either** variable to a non-zero value to opt the Agent out of anonymous telemetry. At container start the entrypoint creates the opt-out marker file, and no anonymous usage statistics are sent.

```bash
docker run -e DISABLE_TELEMETRY=1 netdata/netdata
# or
docker run -e DO_NOT_TRACK=1 netdata/netdata
```

`DO_NOT_TRACK` is an alias for `DISABLE_TELEMETRY`; setting either one is sufficient. See [Anonymous telemetry events](/docs/netdata-agent/configuration/anonymous-telemetry-events.md#opt-out-methods) for full details on what is collected and all the ways to opt out.

### Build-time variables

A few environment variables are baked into the image at build time and are **not** meant to be set with `docker run -e`:

- `NETDATA_OFFICIAL_IMAGE` — marks whether the image is an official Netdata build. It is set during the image build and feeds the anonymous telemetry beacon. You do not need to set it.
- `DOCKER_GRP` — the group name created in the image at build time.

These are documented here for completeness. Changing them at runtime has no supported effect on the entrypoint behavior.

## Publish a test image to your own repository

At Netdata, we provide multiple ways of testing your Docker images using your own repositories.

:::tip

You may either use the command line tools available or take advantage of our GitHub Actions infrastructure.

:::
