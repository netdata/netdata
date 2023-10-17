# Passive journal centralization with encryption using self-signed certificates

This page will guide you through creating a passive journal centralization setup using self-signed certificates for encryption and authorization.

A _passive_ journal server waits for clients to push their metrics to it, so in this setup we will:

1. configure a certificates authority and issue self-signed certificates for your servers.
2. configure `systemd-journal-remote` on the server, to listen for incoming connections.
3. configure `systemd-journal-upload` on the clients, to push their logs to the server.

Keep in mind that the authorization involved works like this:

1. The server (`systemd-journal-remote`) validates that the sender (`systemd-journal-upload`) uses a trusted certificate (a certificate issued by the same certificate authority as its own).
   So, **the server will accept logs from any client having a trusted certificate**.
2. The client (`systemd-journal-upload`) validates that the receiver (`systemd-journal-remote`) uses a trusted certificate (like the server does) and it also checks that the hostname of the URL specified to its configuration, matches one of the names of the server it gets connected to. So, the client does a validation that it connected to the right server, using the URL hostname against the names of the server on its certificate.

This means, that if both certificates are issued by the same certificate authority, only the client can potentially reject the server.

## Self-signed certificates

Use [this script](https://gist.github.com/ktsaou/d62b8a6501cf9a0da94f03cbbb71c5c7) to create a self-signed certificates authority and certificates for all your servers.

```bash
wget -O systemd-journal-self-signed-certs.sh "https://gist.githubusercontent.com/ktsaou/d62b8a6501cf9a0da94f03cbbb71c5c7/raw/c346e61e0a66f45dc4095d254bd23917f0a01bd0/systemd-journal-self-signed-certs.sh"
chmod 755 systemd-journal-self-signed-certs.sh
```

Edit the script and at its top, set your settings:

```bash
# The directory to save the generated certificates (and everything about this certificate authority).
# This is only used on the node generating the certificates (usually on the journals server).
DIR="/etc/ssl/systemd-journal-remote"

# The journals centralization server name (the CN of the server certificate).
SERVER="server-hostname"

# All the DNS names or IPs this server is reachable at (the certificate will include them).
# Journal clients can use any of them to connect to this server.
# systemd-journal-upload validates its URL= hostname, against this list.
SERVER_ALIASES=("DNS:server-hostname1" "DNS:server-hostname2" "IP:1.2.3.4" "IP:10.1.1.1" "IP:172.16.1.1")

# All the names of the journal clients that will be sending logs to the server (the CNs of their certificates).
# These names are used by systemd-journal-remote to name the journal files in /var/log/journal/remote/.
# Also the remote hosts will be presented using these names on Netdata dashboards.
CLIENTS=("vm1" "vm2" "vm3" "add_as_may_as_needed")
```

Then run the script:

```bash
sudo ./systemd-journal-self-signed-certs.sh
```

The script will create the directory `/etc/ssl/systemd-journal-remote` and in it you will find all the certificates needed.

There will also be files named `runme-on-XXX.sh`. There will be 1 script for the server and 1 script for each of the clients.

These `runme-on-XXX.sh` scripts install the needed certificates, fix their file permissions to be accessible by systemd-journal-remote/upload, change `/etc/systemd/journal-remote.conf` (on the server) or `/etc/systemd/journal-upload.conf` (on the clients) and restart the relevant services.

You can copy and paste (or `scp`) these scripts on your server and each of your clients:

```bash
scp /etc/ssl/systemd-journal-remote/runme-on-XXX.sh XXX:/tmp/
```

## Server configuration

On the centralization server install `systemd-journal-remote` and `openssl`:

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote openssl
```

Make sure the journal transfer protocol is `https`:

```bash
sudo cp /lib/systemd/system/systemd-journal-remote.service /etc/systemd/system/

# edit it to make sure it says:
# --listen-https=-3
# not:
# --listen-http=-3
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

Assuming that you have already copied the `runme-on-XXX.sh` script on the server, run this:

```bash
sudo bash /tmp/runme-on-XXX.sh
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts.

## Client configuration

On the clients, install `systemd-journal-remote`:

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Edit `/etc/systemd/journal-upload.conf` and set the IP address and the port of the server, like so:

```conf
[Upload]
URL=https://centralization.server.ip:19532
```

Make sure that `centralization.server.ip` is one of the `SERVER_ALIASES` when you created the certificates.

Edit `systemd-journal-upload`, and add `Restart=always` to make sure the client will keep trying to push logs, even if the server is temporarily not there, like this:

```bash
sudo systemctl edit systemd-journal-upload
```

At the top, add:

```conf
[Service]
Restart=always
```

Enable and start `systemd-journal-upload`, like this:

```bash
sudo systemctl enable systemd-journal-upload
```

Copy the relevant `runme-on-XXX.sh` script as described on server setup and run it:

```bash
sudo bash /tmp/runme-on-XXX.sh
```

The client should now be pushing logs to the central server.


## Verify it works

To verify the central server is receiving logs, run this on the central server:

```bash
sudo ls -l /var/log/journal/remote/
```

You should see new files from the client's canonical names (CN). These are names on the clients' certificates.

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
             └─2118153 /usr/lib/systemd/systemd-journal-remote --listen-https=-3 --output=/var/log/journal/remote/
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
