import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with Docker

## Limitations running the Agent in Docker

We don’t officially support using Docker’s `--user` option or Docker Compose’s `user:` parameter with our images. While they may work, some features could be unavailable. The Agent drops privileges at startup, so most processes don’t run as UID 0 even without these options.  

Additionally, our **POWER8+ Docker images** don’t support the **FreeIPMI collector** due to a technical limitation in FreeIPMI itself, which we can’t work around.

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
| systemd-journal.plugin |         /run/dbus          | Systemd-list-units function: information about all systemd units, including their active state, description, whether they are enabled, and more. |
|      go.d.plugin       |         /run/dbus          | [go.d/systemdunits](https://github.com/netdata/go.d.plugin/tree/master/modules/systemdunits#readme)                                              |

</details>

### Recommended way

Both methods create a [volume](https://docs.docker.com/storage/volumes/) for Netdata's configuration files
_within the container_ at `/etc/netdata`.
See the [configure section](#configure-agent-containers) for details. If you want to access the configuration files from your _host_ machine, see [host-editable configuration](#with-host-editable-configuration).

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

You can restrict access by
following the [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines
to Caddyfile.

### With Docker socket proxy

> **Note:** Using Netdata with a Docker socket proxy may cause some features to not work as expected. It hasn't been fully tested by the Netdata team.

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

Minor and major version tags update with each matching release. For example, if `v1.40.1` is published, the `v1.40` tag moves from `v1.40.0` to `v1.40.1`.

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

## Adding extra packages at runtime

By default, Netdata’s official container images exclude some optional runtime dependencies. You can install them at runtime by setting the `NETDATA_EXTRA_DEB_PACKAGES` environment variable.

Commonly useful packages:  
- `apcupsd` – Monitors APC UPS devices.  
- `lm-sensors` – Monitors hardware sensors.  
- `netcat-openbsd` – Enables IRC alerts.

## Health Checks

Netdata’s Docker image supports **health checks** via standard Docker interfaces. You can control them using the `NETDATA_HEALTHCHECK_TARGET` environment variable:  

- **Unset** – Defaults to checking `/api/v1/info`.  
- **`cli`** – Uses `netdatacli ping` to confirm the Agent is running (but not full data collection).

The default `/api/v1/info` check is usually sufficient. However, if the web server is disabled or API access is restricted, you'll need to customize the health check configuration.

## Publish a test image to your own repository

At Netdata, we provide multiple ways of testing your Docker images using your own repositories.

You may either use the command line tools available or take advantage of our GitHub Actions infrastructure.