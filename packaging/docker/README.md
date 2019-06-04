# Install netdata with Docker

> :warning: As of Sep 9th, 2018 we ship [new docker builds](https://github.com/netdata/netdata/pull/3995), running netdata in docker with an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint) directive, not a COMMAND directive. Please adapt your execution scripts accordingly. You can find more information about ENTRYPOINT vs COMMAND is presented by goinbigdata [here](http://goinbigdata.com/docker-run-vs-cmd-vs-entrypoint/) and by docker docs [here](https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact).
>
> Also, the `latest` is now based on alpine, so **`alpine` is not updated any more** and `armv7hf` is now replaced with `armhf` (to comply with https://github.com/multiarch naming), so **`armv7hf` is not updated** either.

## Limitations

Running netdata in a container for monitoring the whole host, can limit its capabilities. Some data is not accessible or not as detailed as when running netdata on the host.

## Package scrambling in runtime (x86_64 only)

By default on x86_64 architecture our docker images use Polymorphic Polyverse Linux package scrambling. For increased security you can enable rescrambling of packages during runtime. To do this set environment variable `RESCRAMBLE=true` while starting netdata docker container.

For more information go to [Polyverse site](https://polyverse.io/how-it-works/)

## Run netdata with docker command

Quickly start netdata with the docker command line.
Netdata is then available at http://host:19999

This is good for an internal network or to quickly analyse a host.

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

The above can be converted to docker-compose file for ease of management:

```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
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

### Pass command line options to Netdata 

Since we use an [ENTRYPOINT](https://docs.docker.com/engine/reference/builder/#entrypoint) directive, you can provide [netdata daemon command line options](https://docs.netdata.cloud/daemon/#command-line-options) such as the IP address netdata will be running on, using the [command instruction](https://docs.docker.com/engine/reference/builder/#cmd). 

## Install Netdata using Docker Compose with SSL/TLS enabled http proxy

For a permanent installation on a public server, you should [secure the netdata instance](../../docs/netdata-security.md). This section contains an example of how to install netdata with an SSL reverse proxy and basic authentication.

You can use use the following docker-compose.yml and Caddyfile files to run netdata with docker. Replace the Domains and email address for [Letsencrypt](https://letsencrypt.org/) before starting.

### Prerequisites
* [Docker](https://docs.docker.com/install/#server)
* [Docker Compose](https://docs.docker.com/compose/install/)
* Domain configured in DNS pointing to host.

### Caddyfile

This file needs to be placed in /opt with name `Caddyfile`. Here you customize your domain and you need to provide your email address to obtain a Letsencrypt certificate. Certificate renewal will happen automatically and will be executed internally by the caddy server.

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
    image: netdata/netdata
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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Fdocker%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()

## Publish a test image to your own repository

At netdata we provide multiple ways of testing your docker images using your own repositories.
You may either use the command line tools available or take advantage of our Travis CI infrastructure.

### Using tools manually from the command line

The script `packaging/docker/build-test.sh` can be used to create an image and upload it to a repository of your choosing. 

```
Usage: packaging/docker/build-test.sh -r <REPOSITORY> -v <VERSION> -u <DOCKER_USERNAME> -p <DOCKER_PWD> [-s]
	-s skip build, just push the image
Builds an amd64 image and pushes it to the docker hub repository REPOSITORY
```

This is especially useful when testing a Pull Request for Kubernetes, since you can set `image` to an immutable repository and tag, set the `imagePullPolicy` to `Always` and just keep uploading new images.

Example:

We get a local copy of the Helm chart at https://github.com/netdata/helmchart. We modify `values.yaml` to have the following:

```
image:
  repository: cakrit/netdata-prs
  tag: PR5576
  pullPolicy: Always
```

We check out PR5576 and run the following:
```
./packaging/docker/build-test.sh -r cakrit/netdata-prs -v PR5576 -u cakrit -p 'XXX'
```

Then we can run `helm install [path to our helmchart clone]`.

If we make changes to the code, we execute the same `build-test.sh` command, followed by `helm upgrade [name] [path to our helmchart clone]`

### Inside netdata organization, using Travis CI

To enable Travis CI integration on your own repositories (Docker and Github), you need to be part of the Netdata organization.
Once you have contacted the netdata owners to setup you up on Github and Travis, execute the following steps

- Preparation
  - Have netdata forked on your personal GITHUB account
  - Get a GITHUB token: Go to Github settings -> Developer Settings -> Personal access tokens, generate a new token with full access to repo_hook, read only access to admin:org, public_repo, repo_deployment, repo:status and user:email settings enabled. This will be your GITHUB_TOKEN that is described later in the instructions, so keep it somewhere safe until is needed.
  - Contact netdata team and seek for permissions on https://scan.coverity.com should you require Travis to be able to push your forked code to coverity for analysis and report. Once you are setup, you should have your email you used in coverity and a token from them. These will be your COVERITY_SCAN_SUBMIT_EMAIL and COVERITY_SCAN_TOKEN that we will refer to later.
  - Have a valid Docker hub account, the credentials from this account will be your DOCKER_USERNAME and DOCKER_PWD mentioned later

- Setting up Travis CI for your own fork (Detailed instructions provided by Travis team [here](https://docs.travis-ci.com/user/tutorial/))
  - Login to travis with your own GITHUB credentials (There is Open Auth access)
  - Go to your profile settings, under [repositories](https://travis-ci.com/account/repositories) section and setup your netdata fork to be built by travis
  - Once the repository has been setup, go to repository settings within travis (usually under https://travis-ci.com/NETDATA_DEVELOPER/netdata/settings, where "NETDATA_DEVELOPER" is your github handle) and select your desired settings.
- While in Travis settings, under netdata repository settings in the Environment Variables section, you need to add the following:
  - DOCKER_USERNAME and DOCKER_PWD variables so that Travis can login to your docker hub account and publish docker images there. 
  - REPOSITORY variable to "NETDATA_DEVELOPER/netdata" where NETDATA_DEVELOPER is your github handle again.
  - GITHUB_TOKEN variable with the token generated on the preparation step, for travis workflows to function properly
  - COVERITY_SCAN_SUBMIT_EMAIL and COVERITY_SCAN_TOKEN variables to enable Travis to submit your code for analysis to Coverity.

Having followed these instructions, your forked repository should be all set up for Travis Integration, happy testing!
