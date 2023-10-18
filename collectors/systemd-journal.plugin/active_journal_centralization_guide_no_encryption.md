# Active journal centralization without encryption

This page will guide you through creating an active journal centralization setup without the use of encryption.

Once you centralize your infrastructure logs to a server, Netdata will automatically detect all the logs from all
servers and organize them in sources.
With the setup described in this document, journal files are identified by the hostnames of the clients you pull logs.

An _active_ journal server fetch logs from clients, so in this setup we will:

1. configure `systemd-journal-remote` on the server, to pull journal logs.
2. configure `systemd-journal-gatewayd` on the clients, to serve their logs to the micro http server.

> ⚠️ **IMPORTANT**<br/>
> These instructions will copy your logs to a central server, without any encryption or authorization.<br/>
> DO NOT USE THIS ON NON-TRUSTED NETWORKS.

## Client configuration

On the clients, install `systemd-journal-gateway`.

```bash
# change this according to your distro
sudo apt-get install systemd-journal-gateway
```

Optionally, if you want to change the port (the default is `19531`), edit `systemd-journal-gatewayd.socket`

```bash
# edit the socket file
sudo systemctl edit systemd-journal-gatewayd.socket
```

and add the following lines into the instructed place, and choose your desired port; save and exit.

```bash
[Socket]
ListenStream=<DESIRED_PORT>
```

Finally, enable it, so that it will start automatically upon receiving a connection:

```bash
# enable systemd-journal-remote
sudo systemctl daemon-reload 
sudo systemctl enable --now systemd-journal-gatewayd.socket
sudo systemctl enable systemd-journal-gatewayd.service
sudo systemctl start systemd-journal-gatewayd.service
```

## Server configuration

On the centralization server install `systemd-journal-remote`:

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Start it once to make sure than the `systemd-journal-remote` created any necessary requirement to work as centralization
server. To do that, you need to spin up a temporarily _passive_ server with http, then close it, if you won't use it 
also as a passive server.

```bash
sudo cp /lib/systemd/system/systemd-journal-remote.service /etc/systemd/system/

# edit it to make sure it says:
# --listen-http=-3
# not:
# --listen-https=-3
sudo nano /etc/systemd/system/systemd-journal-remote.service

# reload systemd
sudo systemctl daemon-reload
```

Optionally, if you want to change the port (the default is `19532`), edit `systemd-journal-remote.socket`

```bash
# edit the socket file
sudo systemctl edit systemd-journal-remote.socket
```

and add the following lines into the instructed place, and choose your desired port; save and exit.

```bash
[Socket]
ListenStream=<DESIRED_PORT>
```

Start and (stop it, if you won't use it also as _passive_).

```bash
# enable systemd-journal-remote
sudo systemctl start systemd-journal-remote.service
sudo systemctl stop systemd-journal-remote.service
```

For each of your clients (endpoints that you want to fetch journal logs from) create a service that will use
`systemd-journal-remote` will always fetch the logs.


```bash
sudo nano /etc/systemd/system/systemd-journal-endpoint-X.service 
```

Copy the service file above, replace the Description and `TARGET_HOST`, save and exit

```
[Unit]
Description=Fetching systemd journal logs from my endpoint X

[Service]
ExecStart=/usr/lib/systemd/systemd-journal-remote --url http://<TARGET_HOST>:19531/entries?follow
Type=simple
Restart=always
User=systemd-journal-remote

[Install]
WantedBy=multi-user.target
```

Repeat the same for every host that you want to fetch journal logs.
Reload the systemd daemon config, enable each service and start, like this:

```bash
sudo systemctl daemon-reload
sudo systemctl enable systemd-journal-endpoint-X 
sudo systemctl start systemd-journal-endpoint-X
```

## Verify it works

To verify the central server is receiving logs, run this on the central server:

```bash
sudo ls -l /var/log/journal/remote/
```

You should see new files from the client's hostname.

Also, any of the new service files (`systemctl status systemd-journal-endpoint-X`) should show something like this:

```bash
● systemd-journal-client1.service - Fetching systemd journal logs from 192.168.2.146
     Loaded: loaded (/etc/systemd/system/systemd-journal-client1.service; enabled; preset: disabled)
    Drop-In: /usr/lib/systemd/system/service.d
             └─10-timeout-abort.conf
     Active: active (running) since Wed 2023-10-18 07:35:52 EEST; 23min ago
   Main PID: 77959 (systemd-journal)
      Tasks: 2 (limit: 6928)
     Memory: 7.7M
        CPU: 518ms
     CGroup: /system.slice/systemd-journal-client1.service
             ├─77959 /usr/lib/systemd/systemd-journal-remote --url "http://192.168.2.146:19531/entries?follow"
             └─77962 curl "-HAccept: application/vnd.fdo.journal" --silent --show-error "http://192.168.2.146:19531/entries?follow"

Oct 18 07:35:52 systemd-journal-server systemd[1]: Started systemd-journal-client1.service - Fetching systemd journal logs from 192.168.2.146.
Oct 18 07:35:52 systemd-journal-server systemd-journal-remote[77959]: Spawning curl http://192.168.2.146:19531/entries?follow...
```