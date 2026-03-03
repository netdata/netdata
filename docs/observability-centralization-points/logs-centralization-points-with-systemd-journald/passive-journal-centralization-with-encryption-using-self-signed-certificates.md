# Passive journal centralization with encryption using self-signed certificates

This page will guide you through creating a **passive** journal centralization setup using **self-signed certificates** for encryption and authorization.

Once you centralize your infrastructure logs to a server, Netdata will automatically detect all the logs from all servers and organize them in sources. With the setup described in this document, on recent systemd versions, Netdata will automatically name all remote sources using the names of the clients, as they’re described at their certificates (on older versions, the names will be IPs or reverse DNS lookups of the IPs).

A **passive** journal server waits for clients to push their metrics to it, so in this setup we will:

1. configure a certificate authority and issue self-signed certificates for your servers.
2. configure `systemd-journal-remote` on the server, to listen for incoming connections.
3. configure `systemd-journal-upload` on the clients, to push their logs to the server.

Keep in mind that the authorization involved works like this:

1. The server (`systemd-journal-remote`) validates that the client (`systemd-journal-upload`) uses a trusted certificate (a certificate issued by the same certificate authority as its own).
   So, **the server will accept logs from any client having a valid certificate**.
2. The client (`systemd-journal-upload`) validates that the receiver (`systemd-journal-remote`) uses a trusted certificate (like the server does) and it also checks that the hostname or IP of the URL specified to its configuration, matches one of the names or IPs of the server it gets connected to. So, **the client does a validation that it connected to the right server**, using the URL hostname against the names and IPs of the server on its certificate.

This means that if both certificates are issued by the same certificate authority, only the client can potentially reject the server.

## Self-signed certificates

To simplify the process of creating and managing self-signed certificates, we have created [this bash script](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/systemd-journal-self-signed-certs.sh).

This helps to also automate the distribution of the certificates to your servers (it generates a new bash script for each of your servers, which includes everything required, including the certificates).

We suggest keeping this script and all the involved certificates at the journal centralization server, in the directory `/etc/ssl/systemd-journal`, so that you can make future changes as required. If you prefer to keep the certificate authority and all the certificates at a more secure location, use the script on that location.

On the server that will issue the certificates (usually the centralization server), do the following:

```bash
# install systemd-journal-remote to add the users and groups required and openssl for the certs
# change this according to your distro
sudo apt-get install systemd-journal-remote openssl

# download the script and make it executable
curl >systemd-journal-self-signed-certs.sh "https://raw.githubusercontent.com/netdata/netdata/master/src/collectors/systemd-journal.plugin/systemd-journal-self-signed-certs.sh"
chmod 750 systemd-journal-self-signed-certs.sh
```

To create certificates for your servers, run this:

```bash
sudo ./systemd-journal-self-signed-certs.sh "server1" "DNS:hostname1" "IP:10.0.0.1"
```

Where:

- `server1` is the canonical name of the server. On newer systemd version, this name will be used by `systemd-journal-remote` and Netdata when you view the logs on the dashboard.
- `DNS:hostname1` is a DNS name that the server is reachable at. Add `"DNS:xyz"` multiple times to define multiple DNS names for the server.
- `IP:10.0.0.1` is an IP that the server is reachable at. Add `"IP:xyz"` multiple times to define multiple IPs for the server.

Repeat this process to create the certificates for all your servers. You can add servers as required, at any time in the future.

Existing certificates are never re-generated. Typically, certificates need to be revoked and new ones to be issued. But `systemd-journal-remote` tools don’t support handling revocations. So, the only option you have to re-issue a certificate is to delete its files in `/etc/ssl/systemd-journal` and run the script again to create a new one.

Once you run the script of each of your servers, in `/etc/ssl/systemd-journal` you will find shell scripts named `runme-on-XXX.sh`, where `XXX` are the canonical names of your servers.

These `runme-on-XXX.sh` include everything to install the certificates, fix their file permissions to be accessible by `systemd-journal-remote` and `systemd-journal-upload`, and update `/etc/systemd/journal-remote.conf` and `/etc/systemd/journal-upload.conf`.

You can copy and paste (or `scp`) these scripts on your server and each of your clients:

```bash
sudo scp /etc/ssl/systemd-journal/runme-on-XXX.sh XXX:/tmp/
```

For the rest of this guide, we assume that you’ve copied the right `runme-on-XXX.sh` at the `/tmp` of all the servers for which you issued certificates.

### note about certificates file permissions

It is worth noting that `systemd-journal` certificates need to be owned by `systemd-journal-remote:systemd-journal`.

Both the user `systemd-journal-remote` and the group `systemd-journal` are automatically added by the `systemd-journal-remote` package. However, `systemd-journal-upload` (and `systemd-journal-gatewayd` - that is not used in this guide) use dynamic users. Thankfully they’re added to the `systemd-journal` remote group.

So, by having the certificates owned by `systemd-journal-remote:systemd-journal`, satisfies both `systemd-journal-remote` which is not in the `systemd-journal` group, and `systemd-journal-upload` (and `systemd-journal-gatewayd`) which use dynamic users.

You don't need to do anything about it (the scripts take care of everything), but it is worth noting how this works.

## Server configuration

On the centralization server install `systemd-journal-remote`:

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
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

Next, run the `runme-on-XXX.sh` script on the server:

```bash
# if you run the certificate authority on the server:
sudo /etc/ssl/systemd-journal/runme-on-XXX.sh

# if you run the certificate authority elsewhere,
# assuming you have coped the runme-on-XXX.sh script (as described above):
sudo bash /tmp/runme-on-XXX.sh
```

This will install the certificates in `/etc/ssl/systemd-journal`, set the right file permissions, and update `/etc/systemd/journal-remote.conf` and `/etc/systemd/journal-upload.conf` to use the right certificate files.

Finally, enable it, so that it will start automatically upon receiving a connection:

```bash
# enable systemd-journal-remote
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts.

> When done, remember to `rm /tmp/runme-on-*.sh` to make sure your certificates are secure.

## Client configuration

On the clients, install `systemd-journal-remote` (it includes `systemd-journal-upload`):

```bash
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Edit `/etc/systemd/journal-upload.conf` and set the IP address and the port of the server, like so:

```text
[Upload]
URL=https://centralization.server.ip:19532
```

Make sure that `centralization.server.ip` is one of the `DNS:` or `IP:` parameters you defined when you created the centralization server certificates. If it is not, the client may reject to connect.

Next, edit `systemd-journal-upload.service`, and add `Restart=always` to make sure the client will keep trying to push logs, even if the server is temporarily not there, like this:

```bash
sudo systemctl edit systemd-journal-upload.service
```

At the top, add:

```text
[Service]
Restart=always
```

Enable `systemd-journal-upload.service`, like this:

```bash
sudo systemctl enable systemd-journal-upload.service
```

Assuming that you have in `/tmp` the relevant `runme-on-XXX.sh` script for this client, run:

```bash
sudo bash /tmp/runme-on-XXX.sh
```

This will install the certificates in `/etc/ssl/systemd-journal`, set the right file permissions, and update `/etc/systemd/journal-remote.conf` and `/etc/systemd/journal-upload.conf` to use the right certificate files.

Finally, restart `systemd-journal-upload.service`:

```bash
sudo systemctl restart systemd-journal-upload.service
```

The client should now be pushing logs to the central server.

> When done, remember to `rm /tmp/runme-on-*.sh` to make sure your certificates are secure.

Here it is in action, in Netdata:

![2023-10-18 16-23-05](https://github.com/netdata/netdata/assets/2662304/83bec232-4770-455b-8f1c-46b5de5f93a2)

## Verify it works

To verify that the central server is receiving logs, run this on the central server:

```bash
sudo ls -l /var/log/journal/remote/
```

Depending on the `systemd` version you use, you should see new files from the clients' canonical names (as defined at their certificates) or IPs.

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
