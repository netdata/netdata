# Registry Configuration Reference

Netdata utilizes a **central Registry**. Together with certain browser features, it allows Netdata to provide unified cross-server dashboards. Read more about it in the [overview page](/src/registry/README.md).

As presented in the section about [Communication with the Registry](/src/registry/README.md), only your web browser communicates with it, and minimal data is transmitted.

## Run your own Registry

**Every Agent can be a Registry**. You need to use  [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf` and set:

```text
[registry]
    enabled = yes
    registry to announce = http://your.registry:19999
```

Then [restart the Agent](/docs/netdata-agent/start-stop-restart.md) for the changes to take effect.

Afterwards, you need to configure all your other Agents to advertise your Registry to the web browser ([see the Diagram](/src/registry/README.md#communication-with-the-registry)), instead of the default.

Using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf` and set on every Agent:

```text
[registry]
    enabled = no
    registry to announce = http://your.registry:19999
```

You may also want to give your Agents different names under the node menu. You can change their Registry names, by setting on each:

```text
[registry]
    registry hostname = Group1 - Master DB
```

## Limiting access to the Registry

Netdata supports limiting access to the Registry from given IPs, like this:

```text
[registry]
    allow from = *
```

> **Info**
>
> `allow from` settings are [Netdata simple patterns](/src/libnetdata/simple_pattern/README.md). `allow from = !10.1.2.3 10.*` will allow all IPs in `10.*` except `10.1.2.3`.

Keep in mind that connections to Netdata API ports are filtered by `[web].allow connections from`. So, IPs allowed by `[registry].allow from` should also be allowed by `[web].allow connection from`.

The patterns can be matches over IP addresses or FQDN of the host. In order to check the FQDN of the connection without opening the Netdata Agent to DNS-spoofing, a reverse-dns record must be setup for the connecting host. At connection time the reverse-dns of the peer IP address is resolved, and a forward DNS resolution is made to validate the IP address against the name-pattern.

Please note that this process can be expensive on a machine that is serving many connections. The behavior of the pattern matching can be controlled with the following setting:

```text
[registry]
    allow by dns = heuristic
```

Available pattern matching options:

|    name     |                                               description                                                |
|:-----------:|:--------------------------------------------------------------------------------------------------------:|
|    `yes`    |                                  allows the pattern to match DNS names                                   |
|    `no`     |                  disables DNS matching for the patterns (they only match IP addresses)                   |
| `heuristic` | will estimate if the patterns should match FQDNs by the presence or absence of `:`s or alpha-characters. |

## Registry database location

The Registry database is stored at `/var/lib/netdata/registry/*.db`.

There can be up to 2 files:

- `registry-log.db`, the transaction log

    all incoming requests that affect the Registry are saved in this file in real-time.

- `registry.db`, the database

    every `[registry].registry save db every new entries` entries in `registry-log.db`, Netdata will save its database to `registry.db` and empty `registry-log.db`.

Both files are machine readable text files.

## Disable SameSite and Secure cookies

When the Netdata Agent's web server processes a request, it delivers the `SameSite=none` and `Secure` cookies. If you have problems accessing the Agent dashboard or Netdata Cloud, disable these cookies using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf` and setting:

```text
[registry]
    enable cookies SameSite and Secure = no
```

## Troubleshooting the Registry

The Registry URL should be set to the URL of a Netdata dashboard. This server has to have `[registry].enabled = yes`.

So, accessing the Registry URL directly with your web browser, should present the dashboard of the Netdata operating the Registry.

To use the Registry, your web browser needs to support **third party cookies**, since the cookies are set by the Registry while you are browsing the dashboard of another Netdata server. The first time the Registry sees a new web browser it tries to check if the web browser has cookies enabled or not. It does this by setting a cookie and
redirecting the browser back to itself hoping that it will receive the cookie. If it does not receive the cookie, the Registry will keep redirecting your web browser back to itself, which after a few redirects will fail with an error like this:

```text
ERROR 409: Cannot ACCESS netdata registry: https://registry.my-netdata.io responded with: {"status":"redirect","registry":"https://registry.my-netdata.io"}
```

This error is printed on your web browser console (press F12 on your browser to see it).
