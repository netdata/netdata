# Connect Agent to Cloud

This section guides you through installing and securely connecting a new Agent to Netdata Cloud via the encrypted Agent-Cloud Link  ([ACLK](/src/aclk/README.md)). Connecting your Agent to your Space unlocks centralized monitoring, easier collaboration, and more.

## Quick Start - New Installation

**For new installations**, Netdata Cloud generates a command that you can execute on your Node to install and connect the Agent to your Space.

You can find this command in three places in the UI:

- **Space/Room settings**: Click the cogwheel (the bottom-left corner or next to the Room name at the top) and select "Nodes." Click the "+" button to add a new node.
- [**Nodes tab**](/docs/dashboards-and-charts/nodes-tab.md): Click on the "Add nodes" button.
- **Integrations page**: From the "Deploy" groups of integrations, select the OS or container environment your node runs on, and follow the instructions.

## Connect Existing Agent

**For Agents already installed**, you can connect them to your Space using one of three methods:

### Method 1: Via UI (Recommended)

**Best for:** Most users, easiest setup

1. Open your Agent's local dashboard (normally under `IP:19999`)
2. Sign in to your Netdata Cloud account
3. Click the "Connect" button
4. Follow the on-screen instructions to connect your Agent

### Method 2: Via Configuration File

**Best for:** Automated deployments, multiple Agents

Create `/INSTALL_PREFIX/etc/netdata/claim.conf`:

```bash
[global]
   url = https://app.netdata.cloud
   token = NETDATA_CLOUD_SPACE_TOKEN
   rooms = ROOM_KEY1,ROOM_KEY2,ROOM_KEY3
   proxy = http://username:password@myproxy:8080
   insecure = no
```

**Configuration Options:**

|  option  | description                                                                            | required |
|:--------:|:---------------------------------------------------------------------------------------|:--------:|
|   url    | The Netdata Cloud base URL (defaults to `https://app.netdata.cloud`)                   |    no    |
|  token   | The claiming token for your Netdata Cloud Space                                        |   yes    |
|  rooms   | A comma-separated list of Rooms that the Agent will be added to                        |    no    |
|  proxy   | See [proxy configuration](#proxy-configuration) below                                  |    no    |
| insecure | A boolean (either `yes`, or `no`) and when set to `yes` it disables host verification. |    no    |

**Applying the Configuration:**

If the Agent is already running, you can either run `netdatacli reload-claiming-state` or [restart the Agent](/docs/netdata-agent/start-stop-restart.md). Otherwise, the Agent connects when it starts.

### Method 3: Via Environment Variables

**Best for:** Container deployments, CI/CD pipelines

You can configure Netdata using the following environment variables:

| Option                     | Description                                                                                       | Required |
|----------------------------|---------------------------------------------------------------------------------------------------|----------|
| `NETDATA_CLAIM_URL`        | The Netdata Cloud base URL (defaults to `https://app.netdata.cloud`)                              | no       |
| `NETDATA_CLAIM_TOKEN`      | The claiming token for your Netdata Cloud Space                                                   | yes      |
| `NETDATA_CLAIM_ROOMS`      | A comma-separated list of Rooms that the Agent will be added to                                   | no       |
| `NETDATA_CLAIM_PROXY`      | The URL of a proxy server to use for the connection                                               | no       |
| `NETDATA_EXTRA_CLAIM_OPTS` | May contain a space-separated list of options. The option `-insecure` is the only currently used. | no       |

### Connection Troubleshooting

If the connection process fails, you can find the reason in daemon.log (search for "CLAIM") and the `cloud` section of `http://ip:19999/api/v3/info`.

## Advanced Configuration

### Proxy Configuration

You can configure proxy settings for both the configuration file and environment variable methods.

#### For Configuration File (claim.conf)

You can set the `proxy` option at the `[global]` section in `claim.conf` to:

- empty, to disable proxy configuration
- `none` to disable proxy configuration
- `env` to use the environment variable `http_proxy` (this is the default)
- `http://[user:pass@]host:port`, to connect via a web proxy
- `socks5[h]://[user:pass@]host:port`, to connect via a SOCKS5 proxy

#### Environment Variable Proxy Settings

Netdata uses the `http_proxy` environment variable only when you set the `proxy` option to `env` (which is the default). You can set the `http_proxy` environment variable to:

- `http://[user:pass@]host:port`, to connect via an HTTP proxy
- `socks5[h]://[user:pass@]host:port`, to connect via a SOCKS5 or SOCKS5h proxy

#### Proxy Security Considerations

:::note

Netdata does not support secure connections to proxies. **Data between Netdata Agents and Netdata Cloud remains end-to-end encrypted** since the Agent requests a TCP tunnel (HTTP `CONNECT`) from the proxy and handles all encryption directly, however initial Agent-to-proxy communication is not encrypted.

:::

**How End-to-End Encryption Works with Proxies:**

1. **Proxy Connection**: The Agent connects to the HTTP proxy using a plain HTTP connection.
2. **TCP Tunneling Request**: The Agent sends an HTTP CONNECT request to the proxy, asking it to establish a TCP tunnel to the Netdata Cloud server.
3. **Proxy Tunneling**: Once the proxy accepts the CONNECT request (responds with HTTP 200), it creates a TCP tunnel between the Agent and the Netdata Cloud server. At this point, the proxy simply forwards raw TCP data in both directions without interpreting it.
4. **Encrypted Communication**: The Agent then establishes a TLS/SSL connection through this tunnel directly with the Netdata Cloud server. All subsequent data (including the WebSocket handshake and MQTT protocol data) is encrypted end-to-end.

:::note

The proxy only sees encrypted TLS traffic flowing through the tunnel it established, never the decrypted content. This standard method is called "TCP tunneling" or "HTTP CONNECT tunneling."

:::

:::info

Netdata uses **two connection libraries**: **libcurl for claiming and MQTToWSoHTTPS for the actual Cloud connection**. While libcurl supports encrypted proxy connections, MQTToWSoHTTPS does not - so encrypted proxy connections will fail during the Cloud connection phase. The proxy configuration patterns above work for both libraries and provide end-to-end encryption for Netdata Cloud communication.

:::

## Manage Connections

### Reconnect Agent

<details>
<summary><strong>Linux-based Installations</strong></summary><br/>

To remove a node from your Space in Netdata Cloud, delete the `cloud.d/` directory in your Netdata library directory.

```bash
cd /var/lib/netdata   # Replace with your Netdata library directory, if not /var/lib/netdata/
sudo rm -rf cloud.d/
```

:::note

The Agent will be **re-claimed automatically** if the environment variables or `claim.conf` exist when you restart the Agent.

:::

This node will no longer have access to the credentials it used when connecting to Netdata Cloud via the ACLK.

</details>

<details>
<summary><strong>Docker-based Installations</strong></summary><br/>

To remove a node from your Space and connect it to another, follow these steps:

1. **Enter the running container** you wish to remove from your Space

   ```bash
   docker exec -it CONTAINER_NAME sh
   ```

   Replace `CONTAINER_NAME` with either the container's name or ID.

2. **Delete the connection files**

   ```bash
   rm -rf /var/lib/netdata/cloud.d/
   rm /var/lib/netdata/registry/netdata.public.unique.id 
   ```

3. **Stop and remove the container**

   **Docker CLI:**
    ```bash
    docker stop CONTAINER_NAME
    docker rm CONTAINER_NAME
    ```
   Replace `CONTAINER_NAME` with either the container's name or ID.

   **Docker Compose:**  
   Inside the directory that has the `docker-compose.yml` file, run:
    ```bash
    docker compose down
    ```

   **Docker Swarm:**  
   Run the following, and replace `STACK` with your Stack's name:
    ```bash
    docker stack rm STACK
    ```

4. **Connect to new Space**

   Go to your new Space, copy the installation command with the new claim token and run it. If you're using a `docker-compose.yml` file, you will have to overwrite it with the new claiming token. The node should now appear online in that Space.

</details>

### Regenerate Claiming Token

You may need to revoke your previous Claiming Token and generate a new one for security reasons.

:::note

Only **Administrators** of a Space in Netdata Cloud can regenerate Claim Tokens.

:::

**Steps:**

1. Navigate to [any screen](#quick-start---new-installation) containing the Connection command
2. Click the "Regenerate token" button. This action invalidates your previous token and generates a new one

## Troubleshooting

### Check Connection Status

If you're having trouble connecting a node, this may be because the [ACLK](/src/aclk/README.md) cannot connect to Cloud.

**Method 1: Web Interface**

With the Netdata Agent running, visit `http://NODE:19999/api/v3/info` in your browser, replacing `NODE` with the IP address or hostname of your Agent. The returned JSON contains a section called `cloud` with helpful information to diagnose any issues you might be having with the ACLK or connection process.

**Method 2: Command Line**

You can also run `sudo netdatacli aclk-state` to get some diagnostic information about ACLK:

```bash
ACLK Available: Yes
ACLK Implementation: Next Generation
New Cloud Protocol Support: Yes
Claimed: Yes
Claimed Id: 53aa76c2-8af5-448f-849a-b16872cc4ba1
Online: Yes
Used Cloud Protocol: New
```

Use these keys and the information below to troubleshoot the ACLK.

### Common Issues

#### kickstart: unsupported Netdata installation

**Problem:** If you run the kickstart script and get the following error `Existing install appears to be handled manually or through the system package manager.` you most probably installed Netdata using an unsupported package.

**Solution:** Check our [installation section](/packaging/installer/README.md) to find the proper way of installing Netdata on your system.

#### kickstart: Failed to write new machine GUID

**Problem:** You might encounter this error if you run the Netdata kickstart script without sufficient permissions:

```bash
Failed to write new machine GUID. Please make sure you have rights to write to /var/lib/netdata/registry/netdata.public.unique.id.
```

**Solution:** To resolve this issue, you have two options:

1. Run the script with root privileges.
2. Run the script with the user that runs the Netdata Agent.

#### Connecting to Cloud on older distributions (Ubuntu 14.04, Debian 8, CentOS 6)

**Problem:** If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS 6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which helps securely encrypt SSL connections.

**Solution:** We recommend you reinstall Netdata with a [static build](/packaging/installer/methods/kickstart.md#install-type), which uses an up-to-date version of OpenSSL with hostname validation enabled.

:::warning

If you choose to continue using the **outdated version of OpenSSL**, your node will still connect to Netdata Cloud, but **with hostname verification disabled**. Without verification, your Netdata Cloud connection could be vulnerable to man-in-the-middle attacks.

:::
