# Docker container environment variables

The official Netdata Docker image reads a set of environment variables when the container starts. Use them to change the listening port, control health checks, install extra packages at runtime, connect to a Docker socket proxy, or opt out of anonymous telemetry.

Set them with `docker run -e` or the `environment:` block in Docker Compose, the same way you set any container environment variable.

:::tip
For the full installation procedure, required privileges, and volume mounts, see [Install Netdata with Docker](/packaging/docker/README.md). This page is the canonical reference for the environment variables the container's entrypoint honors.
:::

## Reference

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

## NETDATA_LISTENER_PORT

The TCP port the Agent's web server and API listen on. The default is `19999`.

When you change it, remember to publish the same port from the container. For example, to use port `30000`:

```bash
docker run -d --name=netdata \
  -e NETDATA_LISTENER_PORT=30000 \
  -p 30000:30000 \
  netdata/netdata
```

This variable also feeds the default [health check](#netdata_healthcheck_target) target, so the check follows the port automatically unless you override it.

## NETDATA_HEALTHCHECK_TARGET

Controls what the container's Docker `HEALTHCHECK` polls. By default the check requests the Agent's `/api/v1/info` endpoint over the [listener port](#netdata_listener_port): `http://localhost:19999/api/v1/info`.

Two modes:

- **A URL** (the default, or any value other than `cli`): the health check runs `curl` against that URL. The container is healthy if the request succeeds.
- **`cli`**: the health check runs `netdatacli ping`. This confirms the daemon process is alive, but does **not** verify the web server or that data collection is working.

Use `cli` when you have disabled the web server or restricted API access, so the default HTTP check would always report unhealthy:

```bash
docker run -d --name=netdata -e NETDATA_HEALTHCHECK_TARGET=cli netdata/netdata
```

You can also point it at a different endpoint or host when your setup requires it.

## DOCKER_USR

The system user the Agent runs as. The default is `netdata`.

The entrypoint uses this user when assigning supplemental group memberships (the Docker group, the Proxmox configuration group, and the NVIDIA device group), so collectors that need those groups keep working. In normal operation you do not need to change this value.

## DOCKER_HOST and PGID

`DOCKER_HOST` tells Netdata where to find the Docker daemon, and `PGID` is the group ID the Agent needs to read that socket. Both are **auto-detected** at container start.

### Auto-detection

When the container starts as root, the entrypoint probes for a container runtime socket in this order:

1. **balenaEngine** — detected via `/var/run/balena.sock`. When found, the collector is pointed at `unix:///var/run/balena-engine.sock`, and `PGID` is set to that socket's group owner.
2. **Docker** — detected via `/var/run/docker.sock`. When found, the collector is pointed at `unix:///var/run/docker.sock`, and `PGID` is set to that socket's group owner.

The entrypoint then adds the `DOCKER_USR` user to a group with the detected `PGID` so the Agent can read the socket.

### Overriding with a custom value

If neither socket is present, the entrypoint leaves `DOCKER_HOST` and `PGID` untouched, so any value you set is preserved. This is how **Docker socket proxy** setups work: you do not mount `/var/run/docker.sock` into the container, and instead point Netdata at the proxy:

```yaml
environment:
  - DOCKER_HOST=localhost:2375
```

:::warning Detected sockets override your value
If `/var/run/docker.sock` (or `/var/run/balena.sock`) **is** mounted into the container, the entrypoint detects it and overrides any `DOCKER_HOST` and `PGID` you set. To use a custom `DOCKER_HOST` (for example, a socket proxy), do not mount the host socket into the container.
:::

## NETDATA_EXTRA_DEB_PACKAGES

A space-separated list of Debian packages installed with `apt-get` at container start, before the Agent starts. The default is empty (nothing is installed).

This is useful for optional runtime dependencies the image ships without. For example, to enable hardware sensors and IRC alerts:

```bash
docker run -e NETDATA_EXTRA_DEB_PACKAGES="lm-sensors netcat-openbsd" netdata/netdata
```

Installation runs every time the container starts, so adding many packages increases startup time.

## NETDATA_EXTRA_APK_PACKAGES (deprecated)

**Deprecated.** The Netdata Docker image moved from Alpine to Debian as its base. This variable previously installed Alpine (`apk`) packages; it now installs nothing.

If it is set to a non-empty value, the container prints a warning at startup reminding you to use `NETDATA_EXTRA_DEB_PACKAGES` instead, then continues normally.

To silence the warning, unset the variable or set it to an empty string, and migrate your package list to [`NETDATA_EXTRA_DEB_PACKAGES`](#netdata_extra_deb_packages) using Debian package names.

## DISABLE_TELEMETRY and DO_NOT_TRACK

Set **either** variable to a non-zero value to opt the Agent out of anonymous telemetry. At container start the entrypoint creates the opt-out marker file, and no anonymous usage statistics are sent.

```bash
docker run -e DISABLE_TELEMETRY=1 netdata/netdata
# or
docker run -e DO_NOT_TRACK=1 netdata/netdata
```

`DO_NOT_TRACK` is an alias for `DISABLE_TELEMETRY`; setting either one is sufficient. See [Anonymous telemetry events](/docs/netdata-agent/configuration/anonymous-telemetry-events.md#opt-out-methods) for full details on what is collected and all the ways to opt out.

## Build-time variables

A few environment variables are baked into the image at build time and are **not** meant to be set with `docker run -e`:

- `NETDATA_OFFICIAL_IMAGE` — marks whether the image is an official Netdata build. It is set during the image build and feeds system/telemetry information. You do not need to set it.
- `DOCKER_GRP` — the group name created in the image at build time.

These are documented here for completeness. Changing them at runtime has no supported effect on the entrypoint behavior.

## See also

- [Install Netdata with Docker](/packaging/docker/README.md) — installation, privileges, mounts, and update workflow.
- [Anonymous telemetry events](/docs/netdata-agent/configuration/anonymous-telemetry-events.md) — what telemetry collects and how to opt out.
