# Passive journal centralization without encryption

This page will guide you through creating a passive journal centralization setup without the use of encryption.

Once you centralize your infrastructure logs to a server, Netdata will automatically detects all the logs from all servers and organize them in sources.
With the setup described in this document, journal files are identified by the IPs of the clients sending the logs. Netdata will automatically do
reverse DNS lookups to find the names of the server and name the sources on the dashboard accordingly.

A _passive_ journal server waits for clients to push their metrics to it, so in this setup we will:

1. configure `systemd-journal-remote` on the server, to listen for incoming connections.
2. configure `systemd-journal-upload` on the clients, to push their logs to the server.

> ⚠️ **IMPORTANT**<br/>
> These instructions will copy your logs to a central server, without any encryption or authorization.<br/>
> DO NOT USE THIS ON NON-TRUSTED NETWORKS.

## Server configuration

On the centralization server install `systemd-journal-remote`:

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Make sure the journal transfer protocol is `http`:

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

Finally, enable it, so that it will start automatically upon receiving a connection:

```bash
# enable systemd-journal-remote
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts.

## Client configuration

On the clients, install `systemd-journal-remote` (it includes `systemd-journal-upload`):

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Edit `/etc/systemd/journal-upload.conf` and set the IP address and the port of the server, like so:

```text
[Upload]
URL=http://centralization.server.ip:19532
```

Edit `systemd-journal-upload`, and add `Restart=always` to make sure the client will keep trying to push logs, even if the server is temporarily not there, like this:

```bash
sudo systemctl edit systemd-journal-upload
```

At the top, add:

```text
[Service]
Restart=always
```

Enable and start `systemd-journal-upload`, like this:

```bash
sudo systemctl enable systemd-journal-upload
sudo systemctl start systemd-journal-upload
```

## Verify it works

To verify the central server is receiving logs, run this on the central server:

```bash
sudo ls -l /var/log/journal/remote/
```

You should see new files from the client's IP.

Also, `systemctl status systemd-journal-remote` should show something like this:

```bash
systemd-journal-remote.service - Journal Remote Sink Service
     Loaded: loaded (/etc/systemd/system/systemd-journal-remote.service; indirect; preset: disabled)
     Active: active (running) since Sun 2023-10-15 14:29:46 EEST; 2h 24min ago
TriggeredBy: ● systemd-journal-remote.socket
       Docs: man:systemd-journal-remote(8)
             man:journal-remote.conf(5)
   Main PID: 2118153 (systemd-journal)
     Status: "Processing requests..."
      Tasks: 1 (limit: 154152)
     Memory: 2.2M
        CPU: 71ms
     CGroup: /system.slice/systemd-journal-remote.service
             └─2118153 /usr/lib/systemd/systemd-journal-remote --listen-http=-3 --output=/var/log/journal/remote/
```

Note the `status: "Processing requests..."` and the PID under `CGroup`.

On the client `systemctl status systemd-journal-upload` should show something like this:

```bash
● systemd-journal-upload.service - Journal Remote Upload Service
     Loaded: loaded (/lib/systemd/system/systemd-journal-upload.service; enabled; vendor preset: disabled)
    Drop-In: /etc/systemd/system/systemd-journal-upload.service.d
             └─override.conf
     Active: active (running) since Sun 2023-10-15 10:39:04 UTC; 3h 17min ago
       Docs: man:systemd-journal-upload(8)
   Main PID: 4169 (systemd-journal)
     Status: "Processing input..."
      Tasks: 1 (limit: 13868)
     Memory: 3.5M
        CPU: 1.081s
     CGroup: /system.slice/systemd-journal-upload.service
             └─4169 /lib/systemd/systemd-journal-upload --save-state
```

Note the `Status: "Processing input..."` and the PID under `CGroup`.
