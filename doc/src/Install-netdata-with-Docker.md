# Install netdata with Docker

> :warning: As of Sep 9th, 2018 we ship [new docker builds](https://github.com/netdata/netdata/pull/3995), running netdata in docker with an ENTRYPOINT directive, not a COMMAND directive. Please adapt your execution scripts accordingly.
> More information about ENTRYPOINT vs COMMAND is presented by goinbigdata [here](http://goinbigdata.com/docker-run-vs-cmd-vs-entrypoint/) and by docker docs [here](https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact).
>
> Also, the `latest` is now based on alpine, so **`alpine` is not updated any more** and `armv7hf` is now replaced with `armhf` (to comply with https://github.com/multiarch naming), so **`armv7hf` is not updated** either.

## Limitations

Running netdata in a container for monitoring the whole host, can limit its capabilities. Some data is not accessible or not as detailed as when running netdata on the host.

## Run netdata with docker command

Quickly start netdata with the docker command line.
Netdata is then available at http://host:19999

This is good for an internal network or to quickly analyse a host.

For a permanent installation on a public server, you should [[secure the netdata instance|netdata-security]]. See below for an example of how to install netdata with an SSL reverse proxy and basic authentication.

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  firehol/netdata
```

above can be converted to docker-compose file for ease of management:

```yaml
version: '3'
services:
  netdata:
    image: firehol/netdata
    hostname: example.com # set to fqdn of host
    ports:
      - 19999:19999
    cap_add:
      - SYS_PTRACE
    security_opt:
      - apparmor:unconfined
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

### Docker container names resolution

If you want to have your container names resolved by netdata it needs to have access to docker group. To achive that just add environment variable `PGID=999` to netdata container, where `999` is a docker group id from your host. This number can be found by running:
```bash
grep docker /etc/group | cut -d ':' -f 3
```

## Install Netdata using Docker Compose with SSL/TLS enabled http proxy

You can use use the following docker-compose.yml and Caddyfile files to run netdata with docker.
Replace the Domains and email address for Letsencrypt before starting.

### Prerequisites
* [Docker](https://docs.docker.com/install/#server)
* [Docker Compose](https://docs.docker.com/compose/install/)
* Domain configured in DNS pointing to host.

### Caddyfile

This file needs to be placed in /opt with nams Caddyfile. Here you customize your domain and you need to provide your email address to obtain Letsencrypt certificate.
Certificate renewal will happen automatically and will be executed internally by caddy server.

```
netdata.example.org {
  proxy / netdata:19999
  tls admin@example.org
}
```

### docker-compose.yml

After setting Caddyfile run this with `docker-compose up -d` to have fully functioning netdata setup behind HTTP reverse proxy.

```yaml
version: '3'
volumes:
  caddy:

services:
  caddy:
    image: abiosoft/caddy
    ports:
      - 80:80
      - 443:443
    volumes:
      - /opt/Caddyfile:/etc/Caddyfile
      - caddy:/root/.caddy
    environment:
      ACME_AGREE: 'true'
  netdata:
    restart: always
    hostname: netdata.example.org
    image: firehol/netdata
    cap_add:
      - SYS_PTRACE
    security_opt:
      - apparmor:unconfined
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

### Restrict access with basic auth

You can restrict access by following [official caddy guide](https://caddyserver.com/docs/basicauth) and adding lines to Caddyfile.
