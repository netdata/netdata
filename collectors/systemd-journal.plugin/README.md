# `systemd` journal plugin

[KEY FEATURES](#key-features) | [JOURNAL SOURCES](#journal-sources) | [JOURNAL FIELDS](#journal-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[CONFIGURATION](#configuration-and-maintenance) | [FAQ](#faq)

The `systemd` journal plugin by Netdata makes viewing, exploring and analyzing `systemd` journal logs simple and
efficient.
It automatically discovers available journal sources, allows advanced filtering, offers interactive visual
representations and supports exploring the logs of both individual servers and the logs on infrastructure wide
journal centralization servers.

![image](https://github.com/netdata/netdata/assets/2662304/691b7470-ec56-430c-8b81-0c9e49012679)

## Key features

- Works on both **individual servers** and **journal centralization servers**.
- Supports `persistent` and `volatile` journals.
- Supports `system`, `user`, `namespaces` and `remote` journals.
- Allows filtering on **any journal field** or **field value**, for any time-frame.
- Allows **full text search** (`grep`) on all journal fields, for any time-frame.
- Provides a **histogram** for log entries over time, with a break down per field-value, for any field and any
  time-frame.
- Works directly on journal files, without any other third-party components.
- Supports coloring log entries, the same way `journalctl` does.
- In PLAY mode provides the same experience as `journalctl -f`, showing new log entries immediately after they are
  received.

### Prerequisites

`systemd-journal.plugin` is a Netdata Function Plugin.

To protect your privacy, as with all Netdata Functions, a free Netdata Cloud user account is required to access it.
For more information check [this discussion](https://github.com/netdata/netdata/discussions/16136).

### Limitations

#### Plugin availability

The following are limitations related to the availability of the plugin:

- This plugin is not available when Netdata is installed in a container. The problem is that `libsystemd` is not
  available in Alpine Linux (there is a `libsystemd`, but it is a dummy that returns failure on all calls). We plan to
  change this, by shipping Netdata containers based on Debian.
- For the same reason (lack of `systemd` support for Alpine Linux), the plugin is not available on `static` builds of
  Netdata (which are based on `muslc`, not `glibc`).
- On old systemd systems (like Centos 7), the plugin runs always in "full data query" mode, which makes it slower. The
  reason, is that systemd API is missing some important calls we need to use the field indexes of `systemd` journal.
  However, when running in this mode, the plugin offers also negative matches on the data (like filtering for all logs
  that do not have set some field), and this is the reason "full data query" mode is also offered as an option even on
  newer versions of `systemd`.

To use the plugin, install one of our native distribution packages, or install it from source.

#### `systemd` journal features

The following are limitations related to the features of `systemd` journal:

- This plugin does not support binary field values. `systemd` journal has the ability to assign fields with binary data.
  This plugin assumes all fields contain text values (text in this context includes numbers).
- This plugin does not support multiple values per field for any given log entry. `systemd` journal has the ability to
  accept the same field key, multiple times, with multiple values on a single log entry. This plugin will present the
  last value and ignore the others for this log entry.
- This plugin will only read journal files located in `/var/log/journal` or `/run/log/journal`. `systemd-remote` has the
  ability to store journal files anywhere (user configured). If journal files are not located in `/var/log/journal`
  or `/run/log/journal` (and any of their subdirectories), the plugin will not find them.

Other than the above, this plugin supports all features of `systemd` journals.

## Journal Sources

The plugin automatically detects the available journal sources, based on the journal files available in
`/var/log/journal` (persistent logs) and `/run/log/journal` (volatile logs).

![journal-sources](https://github.com/netdata/netdata/assets/2662304/28e63a3e-6809-4586-b3b0-80755f340e31)

The plugin, by default, merges all journal sources together, to provide a unified view of all log messages available.

> To improve query performance, we recommend selecting the relevant journal source, before doing more analysis on the
> logs.

### `system` journals

`system` journals are the default journals available on all `systemd` based systems.

`system` journals contain:

- kernel log messages (via `kmsg`),
- audit records, originating from the kernel audit subsystem,
- messages received by `systemd-journald` via `syslog`,
- messages received via the standard output and error of service units,
- structured messages received via the native journal API.

### `user` journals

Unlike `journalctl`, the Netdata plugin allows viewing, exploring and querying the journal files of **all users**.

By default, each user, with a UID outside the range of system users (0 - 999), dynamic service users,
and the nobody user (65534), will get their own set of `user` journal files. For more information about
this policy check [Users, Groups, UIDs and GIDs on systemd Systems](https://systemd.io/UIDS-GIDS/).

Keep in mind that `user` journals are merged with the `system` journals when they are propagated to a journal
centralization server. So, at the centralization server, the `remote` journals contain both the `system` and `user`
journals of the sender.

### `namespaces` journals

The plugin auto-detects the namespaces available and provides a list of all namespaces at the "sources" list on the UI.

Journal namespaces are both a mechanism for logically isolating the log stream of projects consisting
of one or more services from the rest of the system and a mechanism for improving performance.

`systemd` service units may be assigned to a specific journal namespace through the `LogNamespace=` unit file setting.

Keep in mind that namespaces require special configuration to be propagated to a journal centralization server.
This makes them a little more difficult to handle, from the administration perspective.

### `remote` journals

Remote journals are created by `systemd-journal-remote`. This `systemd` feature allows creating logs centralization
points within your infrastructure, based exclusively on `systemd`.

Usually `remote` journals are named by the IP of the server sending these logs. The Netdata plugin automatically
extracts these IPs and performs a reverse DNS lookup to find their hostnames. When this is successful,
`remote` journals are named by the hostnames of the origin servers.

For information about configuring a journals' centralization server,
check [this FAQ item](#how-do-i-configure-a-journals-centralization-server).

## Journal Fields

`systemd` journals are designed to support multiple fields per log entry. The power of `systemd` journals is that,
unlike other log management systems, it supports dynamic and variable fields for each log message,
while all fields and their values are indexed for fast querying.

This means that each application can log messages annotated with its own unique fields and values, and `systemd`
journals will automatically index all of them, without any configuration or manual action.

For a description of the most frequent fields found in `systemd` journals, check `man systemd.journal-fields`.

Fields found in the journal files are automatically added to the UI in multiple places to help you explore
and filter the data.

The plugin automatically enriches certain fields to make them more user-friendly:

- `_BOOT_ID`: the hex value is annotated with the timestamp of the first message encountered for this boot id.
- `PRIORITY`: the numeric value is replaced with the human-readable name of each priority.
- `SYSLOG_FACILITY`: the encoded value is replaced with the human-readable name of each facility.
- `ERRNO`: the numeric value is annotated with the short name of each value.
- `_UID` `_AUDIT_LOGINUID`, `_SYSTEMD_OWNER_UID`, `OBJECT_UID`, `OBJECT_SYSTEMD_OWNER_UID`, `OBJECT_AUDIT_LOGINUID`:
  the local user database is consulted to annotate them with usernames.
- `_GID`, `OBJECT_GID`: the local group database is consulted to annotate them with group names.
- `_CAP_EFFECTIVE`: the encoded value is annotated with a human-readable list of the linux capabilities.
- `_SOURCE_REALTIME_TIMESTAMP`: the numeric value is annotated with human-readable datetime in UTC.

The values of all other fields are presented as found in the journals.

> IMPORTANT:
> The UID and GID annotations are added during presentation and are taken from the server running the plugin.
> For `remote` sources, the names presented may not reflect the actual user and group names on the origin server.
> The numeric value will still be visible though, as-is on the origin server.

The annotations are not searchable with full-text search. They are only added for the presentation of the fields.

### Journal fields as columns in the table

All journal fields available in the journal files are offered as columns on the UI. Use the gear button above the table:

![image](https://github.com/netdata/netdata/assets/2662304/cd75fb55-6821-43d4-a2aa-033792c7f7ac)

### Journal fields as additional info to each log entry

When you click a log line, the `info` sidebar will open on the right of the screen, to provide the full list of fields
related to this log line. You can close this `info` sidebar, by selecting the filter icon at its top.

![image](https://github.com/netdata/netdata/assets/2662304/3207794c-a61b-444c-8ffe-6c07cbc90ae2)

### Journal fields as filters

The plugin presents a select list of fields as filters to the query, with counters for each of the possible values
for the field. This list can used to quickly check which fields and values are available for the entire time-frame
of the query.

Internally the plugin has:

1. A white-list of fields, to be presented as filters.
2. A black-list of fields, to prevent them from becoming filters. This list includes fields with a very high
   cardinality, like timestamps, unique message ids, etc. This is mainly for protecting the server's performance,
   to avoid building in memory indexes for the fields that almost each of their values is unique.

Keep in mind that the values presented in the filters, and their sorting is affected by the "full data queries"
setting:

![image](https://github.com/netdata/netdata/assets/2662304/ac710d46-07c2-487b-8ce3-e7f767b9ae0f)

When "full data queries" is off, empty values are hidden and cannot be selected. This is due to a limitation of
`libsystemd` that does not allow negative or empty matches. Also, values with zero counters may appear in the list.

When "full data queries" is on, Netdata is applying all filtering to the data (not `libsystemd`), but this means
that all the data of the entire time-frame, without any filtering applied, have to be read by the plugin to prepare
the response required. So, "full data queries" can be significantly slower over long time-frames.

### Journal fields as histogram sources

The plugin presents a histogram of the number of log entries across time.

The data source of this histogram can be any of the fields that are available as filters.
For each of the values this field has, across the entire time-frame of the query, the histogram will get corresponding
dimensions, showing the number of log entries, per value, over time.

The granularity of the histogram is adjusted automatically to have about 150 columns visible on screen.

The histogram presented by the plugin is interactive:

- **Zoom**, either with the global date-time picker, or the zoom tool in the histogram's toolbox.
- **Pan**, either with global date-time picker, or by dragging with the mouse the chart to the left or the right.
- **Click**, to quickly jump to the highlighted point in time in the log entries.

![image](https://github.com/netdata/netdata/assets/2662304/d3dcb1d1-daf4-49cf-9663-91b5b3099c2d)

## PLAY mode

The plugin supports PLAY mode, to continuously update the screen with new log entries found in the journal files.
Just hit the "play" button at the top of the Netdata dashboard screen.

On centralized log servers, PLAY mode provides a unified view of all the new logs encountered across the entire
infrastructure,
from all hosts sending logs to the central logs server via `systemd-remote`.

## Full-text search

The plugin supports searching for any text on all fields of the log entries.

Full text search is combined with the selected filters.

The text box accepts asterisks `*` as wildcards. So, `a*b*c` means match anything that contains `a`, then `b` and
then `c` with anything between them.

## Query performance

Journal files are designed to be accessed by multiple readers and one writer, concurrently.

Readers (like this Netdata plugin), open the journal files and `libsystemd`, behind the scenes, maps regions
of the files into memory, to satisfy each query.

On logs aggregation servers, the performance of the queries depend on the following factors:

1. The **number of files** involved in each query.

   This is why we suggest to select a source when possible.

2. The **speed of the disks** hosting the journal files.

   Journal files perform a lot of reading while querying, so the fastest the disks, the faster the query will finish.

3. The **memory available** for caching parts of the files.

   Increased memory will help the kernel cache the most frequently used parts of the journal files, avoiding disk I/O
   and speeding up queries.

4. The **number of filters** applied.

   Queries are significantly faster when just a few filters are selected.

In general, for a faster experience, **keep a low number of rows within the visible timeframe**.

Even on long timeframes, selecting a couple of filters that will result in a **few dozen thousand** log entries
will provide fast / rapid responses, usually less than a second. To the contrary, viewing timeframes with **millions
of entries** may result in longer delays.

The plugin aborts journal queries when your browser cancels inflight requests. This allows you to work on the UI
while there are background queries running.

At the time of this writing, this Netdata plugin is about 25-30 times faster than `journalctl` on queries that access
multiple journal files, over long time-frames.

During the development of this plugin, we submitted, to `systemd`, a number of patches to improve `journalctl`
performance by a factor of 14:

- https://github.com/systemd/systemd/pull/29365
- https://github.com/systemd/systemd/pull/29366
- https://github.com/systemd/systemd/pull/29261

However, even after these patches are merged, `journalctl` will still be 2x slower than this Netdata plugin,
on multi-journal queries.

The problem lies in the way `libsystemd` handles multi-journal file queries. To overcome this problem,
the Netdata plugin queries each file individually and it then it merges the results to be returned.
This is transparent, thanks to the `facets` library in `libnetdata` that handles on-the-fly indexing, filtering,
and searching of any dataset, independently of its source.

## Configuration and maintenance

This Netdata plugin does not require any configuration or maintenance.

## FAQ

### Can I use this plugin on journals' centralization servers?

Yes. You can centralize your logs using `systemd-journal-remote`, and then install Netdata
on this logs centralization server to explore the logs of all your infrastructure.

This plugin will automatically provide multi-node views of your logs and also give you the ability to combine the logs
of multiple servers, as you see fit.

Check [configuring a logs centralization server](#configuring-a-journals-centralization-server).

### Can I use this plugin from a parent Netdata?

Yes. When your nodes are connected to a Netdata parent, all their functions are available
via the parent's UI. So, from the parent UI, you can access the functions of all your nodes.

Keep in mind that to protect your privacy, in order to access Netdata functions, you need a
free Netdata Cloud account.

### Is any of my data exposed to Netdata Cloud from this plugin?

No. When you access the agent directly, none of your data passes through Netdata Cloud.
You need a free Netdata Cloud account only to verify your identity and enable the use of
Netdata Functions. Once this is done, all the data flow directly from your Netdata agent
to your web browser.

Also check [this discussion](https://github.com/netdata/netdata/discussions/16136).

When you access Netdata via `https://app.netdata.cloud`, your data travel via Netdata Cloud,
but they are not stored in Netdata Cloud. This is to allow you access your Netdata agents from
anywhere. All communication from/to Netdata Cloud is encrypted.

### What are `volatile` and `persistent` journals?

`systemd` `journald` allows creating both `volatile` journals in a `tmpfs` ram drive,
and `persistent` journals stored on disk.

`volatile` journals are particularly useful when the system monitored is sensitive to
disk I/O, or does not have any writable disks at all.

For more information check `man systemd-journald`.

### I centralize my logs with Loki. Why to use Netdata for my journals?

`systemd` journals have almost infinite cardinality at their labels and all of them are indexed,
even if every single message has unique fields and values.

When you send `systemd` journal logs to Loki, even if you use the `relabel_rules` argument to
`loki.source.journal` with a JSON format, you need to specify which of the fields from journald
you want inherited by Loki. This means you need to know the most important fields beforehand.
At the same time you loose all the flexibility `systemd` journal provides:
**indexing on all fields and all their values**.

Loki generally assumes that all logs are like a table. All entries in a stream share the same
fields. But journald does exactly the opposite. Each log entry is unique and may have its own unique fields.

So, Loki and `systemd-journal` are good for different use cases.

`systemd-journal` already runs in your systems. You use it today. It is there inside all your systems
collecting the system and app~~~~lications logs. And for its use case, it has advantages over other
centralization solutions. So, why not use it?

### Is it worth to build a `systemd` logs centralization server?

Yes. It is simple, fast and the software to do it is already in your systems.

For application and system logs, `systemd` journal is ideal and the visibility you can get
by centralizing your system logs and the use of this Netdata plugin, is unparalleled.

### How do I configure a journals' centralization server?

A short summary to get journal server running can be found below.
There are two strategies you can apply, when it comes down to a centralized server for systemd journal logs.

1. _Active sources_, where the centralized server fetches the logs from each individual server
2. _Passive sources_, where the centralized server accepts a log stream from an individual server.

For more options and reference to documentation, check `man systemd-journal-remote` and `man systemd-journal-upload`.

We will focus on providing some instructions on setting up a _passive_ centralized server.

⚠️ Two things to keep always in mind:

1. `systemd-journal-remote` doesn't provide a mechanism to authorize each individual server to write its logs to
   the parent server. Especially in public-faced servers you need to make sure that the endpoints of this service
   are protected from "bad actors" (for instance; on the centralization server, allow traffic to the
   `systemd-journal-remote` specific port (`19532`) only from each individual server)
2. Even with TLS enabled on the centralization server, we don't advise you to push systemd journal logs over the public
   network. Prefer cleaner approaches, for instance, create one centralization server per one specific subnet of your
   VPC.

#### Configuring a journals' centralization server

On the centralization server install `systemd-journal-remote`, and enable it with `systemctl`, like this:

```sh
# change this according to your distro
sudo apt-get install systemd-journal-remote

# enable receiving
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts, on port `19532`.
Please note that `systemd-journal-remote` supports using secure connections.

##### Centralization server without TLS (use case; only for secure intranets)

1. To change the protocol of the journal transfer (from HTTPS, which is the default to HTTP), edit the service file of
   the `systemd-journal-remote` service.

    ```sh
    sudo cp /lib/systemd/system/systemd-journal-remote.service /etc/systemd/system/
    
    # edit it
    # --listen-http=-3 specifies the incoming journal for http.
    # If you want to use https, change it to --listen-https=-3.
    nano /etc/systemd/system/systemd-journal-remote.service
    
    # reload systemd
    sudo systemctl daemon-reload
    ```

   This will make HTTP requests as priority.

2. Reload the daemon configs

    ```sh
    # reload systemd
    sudo systemctl daemon-reload
    ```

3. (OPTIONAL) If you want to change the port, edit the socket file of the `systemd-journal-remote`

    ```sh
    # copy the service file
    sudo systemctl edit systemd-journal-remote.socket
    ```

   and add the following lines into the instructed place, and choose your desired port; save and exit.

    ```sh
    [Socket]
    ListenStream=<DESIRED_PORT>
    ```


4. Secure the endpoint from unauthorized access. That depends on your setup (e.g firewall setting, reverse proxies, etc)

##### Centralization server with TLS and self-signed certificate

Follow **all** the steps from
the [Centralization server without TLS (use case; only in secure intranets](#centralization-server-without-tls--use-case-only-in-secure-intranets-)
but omit step 2. Instead of step two, take the following steps:

> 💡 You need to handcraft and use a self-signed certificate. A pretty straightforward way to do that is:

1. Download OpenSSL

    ```sh
    # change this according to your distro
    sudo apt-get install openssl
    ```

2. Create your own private certificate authority.

    ```sh
    mkdir self-signed-certificates && cd self-signed-certificates
    
    openssl req -newkey rsa:2048 -days 3650 -x509 -nodes \
          -out ca.pem -keyout ca.key -subj '/CN=My Certificate authority/'
          
    cat >ca.conf <<EOF
    [ ca ]
    default_ca = CA_default
    [ CA_default ]
    new_certs_dir = .
    certificate = ca.pem
    database = ./index
    private_key = ca.key
    serial = ./serial
    default_days = 3650
    default_md = default
    policy = policy_anything
    [ policy_anything ]
    countryName             = optional
    stateOrProvinceName     = optional
    localityName            = optional
    organizationName        = optional
    organizationalUnitName  = optional
    commonName              = supplied
    emailAddress            = optional
    EOF
    
    touch index
    echo 0001 >serial
    ```

3. Specify the Common Names for both the server and the clients (server who will push its journal logs).

   How each a client will reach the centralized the server? For instance if you want to reach them via public IP or DNS.
   There is a 1:1 correlation among the elements of `CLIENT_CNS`, `CLIENT_IPS` and `CLIENT_DNES`, if you want to omit
   a field, replace it with a placeholder string. You can also omit the DNSes but keep in mind to also omit the field
   `DNS:$SERVER_DNS` when you are signing the client certificates.

    ```sh
    SERVER_CN="myserver.example.com"
    SERVER_IP="Server_IP"
    SERVER_DNS="myserver.example.com"
    
    CLIENT_CNS=("client1.example.com" "client2.example.com")
    CLIENT_IPS=("client1_ip" "client2_ip")
    CLIENT_DNES=("client1.example.com" "client2.example.com")
    ```

4. Create the self-signed certificates for the server and the clients

    ```sh
    openssl req -newkey rsa:2048 -nodes -out $SERVER_CN.csr -keyout $SERVER_CN.key -subj "/CN=$SERVER_CN/"
    echo "subjectAltName = IP:$SERVER_IP, DNS:$SERVER_DNS" > $SERVER_CN.ext
    openssl ca -batch -config ca.conf -notext -in $SERVER_CN.csr -out $SERVER_CN.pem -extfile $SERVER_CN.ext
    
    for i in "${!CLIENT_CNS[@]}"; do
          CLIENT_CN="${CLIENT_CNS[$i]}"
          CLIENT_IP="${CLIENT_IPS[$i]}"
          CLIENT_DNS="${CLIENT_DNES[$i]}"
          # Generate the client CSR
          openssl req -newkey rsa:2048 -nodes -out $CLIENT_CN.csr -keyout $CLIENT_CN.key -subj "/CN=$CLIENT_CN/"
          echo "subjectAltName = IP:$CLIENT_IP" > $CLIENT_CN.ext
    
          # Sign the client certificate using the CA configuration
          openssl ca -batch -config ca.conf -notext -in $CLIENT_CN.csr -out $CLIENT_CN.pem -extfile $CLIENT_CN.ext
    done
    ```

   Keep in mind we have already produced the client certificates, we will make use of them when we will configure the
   clients.

5. Copy the key and the certificates into the `systemd-journal-remote`'s predefined places

    ```sh
    sudo mkdir /etc/ssl/private # make sure that you havent created this folder, and use it already, you may dont want to change it's permissions
    sudo chmod 755 /etc/ssl/private
    sudo mkdir /etc/ssl/ca/ # make sure that you havent created this folder, and use it already, you may dont want to change it's permissions
    sudo chmod 755 /etc/ssl/ca
    sudo cp "${SERVER_CN}".key /etc/ssl/private/journal-remote.key # This is not predefined but we need to clarify the key
    sudo cp "${SERVER_CN}".pem /etc/ssl/certs/journal-remote.pem
    sudo cp ca.pem /etc/ssl/ca/trusted.pem
    ```

6. Adjust the permissions for the `systemd-journal-remote` to access them.

    ```sh
    sudo chgrp systemd-journal-remote /etc/ssl/private/journal-remote.key
    sudo chgrp systemd-journal-remote /etc/ssl/certs/journal-remote.pem
    sudo chgrp systemd-journal-remote /etc/ssl/ca/trusted.pem
    
    
    sudo chmod 0640 /etc/ssl/private/journal-remote.key
    sudo chmod 755 /etc/ssl/certs/journal-remote.pem
    sudo chmod 755 /etc/ssl/ca/trusted.pem
    ```

7. Edit the `systemd-journal-remote.conf` to change the predefined key place and enable SSL.

    ```sh
    sudo nano /etc/systemd/journal-remote.conf
    ```

   You need to transform the corresponding section to something like this
    ```
    [Remote]
    Seal=false
    SplitMode=host
    ServerKeyFile=/etc/ssl/private/journal-remote.key
    ServerCertificateFile=/etc/ssl/certs/journal-remote.pem
    TrustedCertificateFile=/etc/ssl/ca/trusted.pem
    ```

#### Configuring journal clients to push their logs to the server

In this section we will configure the clients/hosts to push their journal logs into the centralization server. You will
install `systemd-journal-remote`,
configure `systemd-journal-upload` (with or without SSL), enable and start it.

1. To install `systemd-journal-remote`, run:

    ```sh
    # change this according to your distro
    sudo apt-get install systemd-journal-remote
    ```


2. **With SSL**: Copy to the client/hosts the self-signed certificates you created before (you created one per host)

   After this step, each host must have the following inside a directory (e.g. `home/user/incoming`),
    ```
    clientX.example.com.key
    clientX.example.com.pem
    ca.pem #common between the servers
    ```

3. **With SSL**: _On each client/host;_ create a user and a group (with the same name) called `systemd-journal-upload`

   ```sh
   sudo adduser --system --home /run/systemd --no-create-home --disabled-login --group systemd-journal-upload
   ```

4. **With SSL**: _On each client/host;_ Navigate under the directory you placed the certificates, copy them into the
   expected locations (by the `systemd-journal-upload` service)

    ```sh
    sudo mkdir /etc/ssl/private # make sure that you havent created this folder, and use it already, you may dont want to change it's permissions.
    sudo chmod 755 /etc/ssl/private
    sudo mkdir /etc/ssl/ca/ # make sure that you havent created this folder, and use it already, you may dont want to change it's permissions.
    sudo chmod 755 /etc/ssl/ca
    
    cd home/user/incoming #change it accordingly, this is the place where you copied your certificates.
    sudo cp clientX.example.com.key /etc/ssl/private/journal-upload.key
    sudo cp clientX.example.com.pem /etc/ssl/certs/journal-upload.pem
    sudo cp ca.pem /etc/ssl/ca/trusted.pem
    ```

5. **With SSL**:  _On each client/host;_ Adjust the permission so that the `systemd-journal-upload` service can access
   the files

    ```sh
    sudo chgrp systemd-journal-upload /etc/ssl/private/journal-upload.key
    sudo chgrp systemd-journal-upload /etc/ssl/certs/journal-upload.pem
    sudo chgrp systemd-journal-upload /etc/ssl/ca/trusted.pem
    
    sudo chmod 0640 /etc/ssl/private/journal-upload.key
    sudo chmod 755 /etc/ssl/certs/journal-upload.pem
    sudo chmod 755 /etc/ssl/ca/trusted.pem
    ```

6. Edit `/etc/systemd/journal-upload.conf` and set the IP address and the port of the server, like so:

    ```
    [Upload]
    URL=http://centralization.server.ip:19532
    ```

   ⚠️ OR with SSL

    ```
    [Upload]
    URL=https://CENTRALIZED_SERVER_IP/DOMAIN :19532 #replace it accordingly
    ServerKeyFile=/etc/ssl/private/journal-upload.key
    ServerCertificateFile=/etc/ssl/certs/journal-upload.pem
    TrustedCertificateFile=/etc/ssl/ca/trusted.pem
    ```

7. Edit `systemd-journal-upload`, and add `Restart=always` to make sure the client will keep trying to push logs, even
   if the server is temporarily not there, like this:

    ```sh
    sudo systemctl edit systemd-journal-upload
    ```

   At the top, add:

    ```
    [Service]
    Restart=always
    ```

8. Enable and start `systemd-journal-upload`, like this:

    ```sh
    sudo systemctl enable systemd-journal-upload
    sudo systemctl start systemd-journal-upload
    ```

Keep in mind that immediately after starting `systemd-journal-upload` on a server, a replication process starts pushing
logs in the order they have been received. This means that depending on the size of the available logs, some time may be
needed for Netdata to show the most recent logs of that server.

#### Limitations when using a logs centralization server

As of this writing `namespaces` support by `systemd` is limited:

- Docker containers cannot log to namespaces. Check [this issue](https://github.com/moby/moby/issues/41879).
- `systemd-journal-upload` automatically uploads `system` and `user` journals, but not `namespaces` journals. For this
  you need to spawn a `systemd-journal-upload` per namespace.

