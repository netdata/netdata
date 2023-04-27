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

## Limitations running the Agent in Docker

For monitoring the whole host, running the Agent in a container can limit its capabilities. Some data, like the host OS
performance or status, is not accessible or not as detailed in a container as when running the Agent directly on the
host.

A way around this is to provide special mounts to the Docker container so that the Agent can get visibility on host OS
information like `/sys` and `/proc` folders or even `/etc/group` and shadow files.

Also, we now ship Docker images using an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint)
directive, not a COMMAND directive. Please adapt your execution scripts accordingly. You can find more information about
ENTRYPOINT vs COMMAND in the [Docker
documentation](https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact).

Our POWER8+ Docker images do not support our FreeIPMI collector. This is a technical limitation in FreeIPMI itself,
and unfortunately not something we can realistically work around.

## Create a new Netdata Agent container

You can create a new Agent container using either `docker run` or `docker-compose`. After using either method, you can
visit the Agent dashboard `http://NODE:19999`.

Both methods create a [bind mount](https://docs.docker.com/storage/bind-mounts/) for Netdata's configuration files
_within the container_ at `/etc/netdata`. See the [configuration section](#configure-agent-containers) for details. If
you want to access the configuration files from your _host_ machine, see [host-editable
configuration](#host-editable-configuration).

<Tabs>
<TabItem value="docker_run" label="docker run">

<h3> Using the <code>docker run</code> command </h3>

Run the following command along with the following options on your terminal, to start a new container.

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

> ### Note
>  
> If you plan to Claim the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's Nodes tab.

</TabItem>
<TabItem value="docker compose" label="docker-compose">

<h3> Using the <code>docker-compose</code> command</h3>

#### Steps

1. Copy the following code and paste into a new file called `docker-compose.yml`

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

2. Run `docker-compose up -d` in the same directory as the `docker-compose.yml` file to start the container.

> :bookmark_tabs: Note
>  
> If you plan to Claim the node to Netdata Cloud, you can find the command with the right parameters by clicking the "Add Nodes" button in your Space's "Nodes" view.

</TabItem>
</Tabs>

## Docker tags

See our full list of Docker images at [Docker Hub](https://hub.docker.com/r/netdata/netdata).

The official `netdata/netdata` Docker image provides the following named tags:

* `stable`: The `stable` tag will always point to the most recently published stable build.
* `edge`: The `edge` tag will always point ot the most recently published nightly build. In most cases, this is
  updated daily at around 01:00 UTC.
* `latest`: The `latest` tag will always point to the most recently published build, whether it’s a stable build
  or a nightly build. This is what Docker will use by default if you do not specify a tag.

Additionally, for each stable release, three tags are pushed, one with the full version of the release (for example,
`v1.30.0`), one with just the major and minor version (for example, `v1.30`), and one with just the major version
(for example, `v1`). The tags for the minor versions and major versions are updated whenever a release is published
that would match that tag (for example, if `v1.30.1` were to be published, the `v1.30` tag would be updated to
point to that instead of `v1.30.0`).

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

-   If left unset, the health check will attempt to access the
    `/api/v1/info` endpoint of the agent.
-   If set to the exact value 'cli', the health check
    script will use `netdatacli ping` to determine if the agent is running
    correctly or not. This is sufficient to ensure that Netdata did not
    hang during startup, but does not provide a rigorous verification
    that the daemon is collecting data or is otherwise usable.
-   If set to anything else, the health check will treat the value as a
    URL to check for a 200 status code on. In most cases, this should
    start with `http://localhost:19999/` to check the agent running in
    the container.

In most cases, the default behavior of checking the `/api/v1/info`
endpoint will be sufficient. If you are using a configuration which
disables the web server or restricts access to certain APIs, you will
need to use a non-default configuration for health checks to work.

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

### Host-editable configuration

> :warning: Warning
>  
> The [edit-config](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) script doesn't work when executed on
> the host system.

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

**`docker run`**: Use the `docker run` command, along with the following options, to start a new container. Note the
changed `-v $(pwd)/netdataconfig/netdata:/etc/netdata \` line from the recommended example above.

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v $(pwd)/netdataconfig/netdata:/etc/netdata \
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

**Docker Compose**: Copy the following code and paste into a new file called `docker-compose.yml`, then run
`docker-compose up -d` in the same directory as the `docker-compose.yml` file to start the container. Note the changed
`./netdataconfig/netdata:/etc/netdata:ro` line from the recommended example above.

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

### Change the default hostname

You can change the hostname of a Docker container, and thus the name that appears in the local dashboard and in Netdata
Cloud, when creating a new container. If you want to change the hostname of a Netdata container _after_ you started it,
you can safely stop and remove it. Your configuration and metrics data reside in persistent volumes and are reattached to
the recreated container.

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

Alternatively, you can directly use the hostname from the node running the container by mounting
`/etc/hostname` from the host in the container. With `docker run`, this can be done by adding `--volume
/etc/hostname:/etc/hostname:ro` to the options. If you are using Docker Compose, you can add an entry to the
container's `volumes` section reading `- /etc/hostname:/etc/hostname:ro`.

### Add or remove other volumes

Some volumes are optional depending on how you use Netdata:

-   If you don't want to use the apps.plugin functionality, you can remove the mounts of `/etc/passwd` and `/etc/group`
    (they are used to get proper user and group names for the monitored host) to get slightly better security.
-   Most modern linux distros supply `/etc/os-release` although some older distros only supply `/etc/lsb-release`. If
    this is the case you can change the line above that mounts the file inside the container to
    `-v /etc/lsb-release:/host/etc/lsb-release:ro`.
-   If your host is virtualized then Netdata cannot detect it from inside the container and will output the wrong
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

#### Files inside systemd volumes

If a volume is used by systemd service, some files can be removed during 
[reinitialization](https://github.com/netdata/netdata/issues/9916). To avoid this, you need to add
`RuntimeDirectoryPreserve=yes` to the service file.

### Docker container names resolution

There are a few options for resolving container names within Netdata. Some methods of doing so will allow root access to
your machine from within the container. Please read the following carefully.

#### Docker socket proxy (safest option)

Deploy a Docker socket proxy that accepts and filters out requests using something like
[HAProxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md) or
[CetusGuard](https://github.com/hectorm/cetusguard) so that it restricts connections to read-only access to the `/containers`
endpoint.

The reason it's safer to expose the socket to the proxy is because Netdata has a TCP port exposed outside the Docker
network. Access to the proxy container is limited to only within the network.

Here are two examples, the first using [a Docker image based on HAProxy](https://github.com/Tecnativa/docker-socket-proxy)
and the second using [CetusGuard](https://github.com/hectorm/cetusguard).

##### Docker Socket Proxy (HAProxy)

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

##### CetusGuard

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    # ... rest of your config ...
    ports:
      - 19999:19999
    environment:
      - DOCKER_HOST=cetusguard:2375
  cetusguard:
    image: hectorm/cetusguard:v1
    read_only: true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      CETUSGUARD_BACKEND_ADDR: unix:///var/run/docker.sock
      CETUSGUARD_FRONTEND_ADDR: tcp://:2375
      CETUSGUARD_RULES: |
        ! Inspect a container
        GET %API_PREFIX_CONTAINERS%/%CONTAINER_ID_OR_NAME%/json
```

You can run the socket proxy in its own Docker Compose file and leave it on a private network that you can add to
other services that require access.

#### Giving group access to the Docker socket (less safe)

> :warning: Caution
>  
> You should seriously consider the necessity of activating this option, as it grants to the `netdata`
> user access to the privileged socket connection of docker service and therefore your whole machine.

If you want to have your container names resolved by Netdata, make the `netdata` user be part of the group that owns the
socket.

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

#### Running as root (unsafe)

> :warning: Caution
>  
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

### Docker container network interfaces monitoring

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

### Pass command line options to Netdata

Since we use an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint) directive, you can provide
[Netdata daemon command line options](https://github.com/netdata/netdata/blob/master/daemon/README.md#command-line-options) such as the IP address Netdata will be
running on, using the [command instruction](https://docs.docker.com/engine/reference/builder/#cmd). 

## Install the Agent using Docker Compose with SSL/TLS enabled HTTP Proxy

For a permanent installation on a public server, you should [secure the Netdata
instance](https://github.com/netdata/netdata/blob/master/docs/netdata-security.md). This section contains an example of how to install Netdata with an SSL
reverse proxy and basic authentication.

You can use the following `docker-compose.yml` and Caddyfile files to run Netdata with Docker. Replace the domains and
email address for [Let's Encrypt](https://letsencrypt.org/) before starting.

### Caddyfile

This file needs to be placed in `/opt` with name `Caddyfile`. Here you customize your domain, and you need to provide
your email address to obtain a Let's Encrypt certificate. Certificate renewal will happen automatically and will be
executed internally by the caddy server.

```caddyfile
netdata.example.org {
  reverse_proxy netdata:19999
  tls admin@example.org
}
```

### docker-compose.yml

After setting Caddyfile run this with `docker-compose up -d` to have fully functioning Netdata setup behind HTTP reverse
proxy.

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

### Restrict access with basic auth

You can restrict access by following [official caddy guide](https://caddyserver.com/docs/caddyfile/directives/basicauth#basicauth) and adding lines to
Caddyfile.

## Publish a test image to your own repository

At Netdata, we provide multiple ways of testing your Docker images using your own repositories.
You may either use the command line tools available or take advantage of our GitHub Actions infrastructure.
