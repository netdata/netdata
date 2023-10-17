# Passive journal centralization with encryption using self-signed certificates

This page will guide you through creating a passive journal centralization setup using self-signed certificates for encryption.

> A _passive_ journal server waits for clients to push their metrics to it.

## Server configuration

On the centralization server install `systemd-journal-remote` and `openssl`:

```sh
# change this according to your distro
sudo apt-get install systemd-journal-remote openssl
```

Make sure the journal transfer protocol is `https`:

```sh
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

```sh
# edit the socket file
sudo systemctl edit systemd-journal-remote.socket
```

and add the following lines into the instructed place, and choose your desired port; save and exit.

```sh
[Socket]
ListenStream=<DESIRED_PORT>
```

Finally, enable it, so that it will start automatically upon receiving a connection:

```sh
# enable systemd-journal-remote
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts.

Use [this script](https://gist.github.com/ktsaou/d62b8a6501cf9a0da94f03cbbb71c5c7) to create a self-signed certificates authority and certificates for all your servers.

```sh
wget -O systemd-journal-self-signed-certs.sh "https://gist.githubusercontent.com/ktsaou/d62b8a6501cf9a0da94f03cbbb71c5c7/raw/c346e61e0a66f45dc4095d254bd23917f0a01bd0/systemd-journal-self-signed-certs.sh"
chmod 755 systemd-journal-self-signed-certs.sh
```

Edit the script and at its top, set your settings:

```sh
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

```sh
sudo ./systemd-journal-self-signed-certs.sh
```

The script will create the directory `/etc/ssl/systemd-journal-remote` and in it you will find all the certificates needed.

There will also be files named `runme-on-XXX.sh`. There will be 1 script for the server and 1 script for each of the clients. You can copy and paste (or `scp`) these scripts on your server and each of your clients and run them as root:

```sh
scp /etc/ssl/systemd-journal-remote/runme-on-XXX.sh XXX:/tmp/
```

Once the above is done, `ssh` to each server/client and do:

```sh
sudo bash /tmp/runme-on-XXX.sh
```

The scripts install the needed certificates, fix their file permissions to be accessible by systemd-journal-remote/upload, change `/etc/systemd/journal-remote.conf` (on the server) or `/etc/systemd/journal-upload.conf` on the clients and restart the relevant services.

## Client configuration

On the clients, install `systemd-journal-remote`:

```sh
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

```sh
sudo systemctl edit systemd-journal-upload
```

At the top, add:

```conf
[Service]
Restart=always
```

Enable and start `systemd-journal-upload`, like this:

```sh
sudo systemctl enable systemd-journal-upload
```

Copy the relevant `runme-on-XXX.sh` script as described on server setup and run it:

```sh
sudo bash /tmp/runme-on-XXX.sh
```
