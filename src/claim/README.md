# Connect Agent to Cloud

This section guides you through installing and securely connecting a new Agent to Netdata Cloud via the encrypted Agent-Cloud Link ([ACLK](/src/aclk/README.md)). Connecting your Agent to your Space in Netdata Cloud unlocks additional features like centralized monitoring and easier collaboration.

## Connect

### Install and Connect a New Agent

There are three places in the UI where you can add/connect your Node:

- **Space/Room settings**: Click the cogwheel (the bottom-left corner or next to the Room name at the top) and select "Nodes." Click the "+" button to add a new node.
- [**Nodes tab**](/docs/dashboards-and-charts/nodes-tab.md): Click on the "Add nodes" button.
- **Integrations page**: From the "Deploy" groups of integrations, select the OS or container environment your node runs on, and follow the instructions.

Netdata Cloud will generate a command that you can execute on your Node to install and connect the Agent to your Space.

### Connect an existing Agent

There are three methods to connect an already installed Agent to your Space.

#### Manually, via the UI

The UI method is the easiest and recommended way to connect your Agent. Here's how:

1. Open your Agent's local dashboard (normally under `IP:19999`).
2. Sign in to your Netdata Cloud account.
3. Click the "Connect" button.
4. Follow the on-screen instructions to connect your Agent.

#### Automatically, via a provisioning system or the command line

Netdata Agents can be connected to Netdata Cloud by creating `/INSTALL_PREFIX/etc/netdata/claim.conf`:

```bash
[global]
   url = https://app.netdata.cloud
   token = NETDATA_CLOUD_SPACE_TOKEN
   rooms = ROOM_KEY1,ROOM_KEY2,ROOM_KEY3
   proxy = http://username:password@myproxy:8080
   insecure = no
```

|  option  | description                                                                            | required |
|:--------:|:---------------------------------------------------------------------------------------|:--------:|
|   url    | The Netdata Cloud base URL (defaults to `https://app.netdata.cloud`)                   |    no    |
|  token   | The claiming token for your Netdata Cloud Space                                        |   yes    |
|  rooms   | A comma-separated list of Rooms that the Agent will be added to                        |    no    |
|  proxy   | Check below for possible values                                                        |    no    |
| insecure | A boolean (either `yes`, or `no`) and when set to `yes` it disables host verification. |    no    |

If the Agent is already running, you can either run `netdatacli reload-claiming-state` or [restart the Agent](/docs/netdata-agent/start-stop-restart.md). Otherwise, the Agent will be connected when it starts.

If the connection process fails, the reason will be logged in daemon.log (search for "CLAIM") and the `cloud` section of `http://ip:19999/api/v3/info`.

##### Proxy configuration for claiming via claim.conf

The `proxy` option at the `[global]` section in `claim.conf` can be set to:

- empty, to disable proxy configuration.
- `none` to disable proxy configuration.
- `env` to use the environment variable `http_proxy` (this is the default).
- `http://[user:pass@]host:port`, to connect via a web proxy.
- `socks5[h]://[user:pass@]host:port`, to connect via a SOCKS5 proxy.

The `http_proxy` environment variable is used only when the `proxy` option is set to `env` (which is the default). The `http_proxy` environment can be:

- `http://[user:pass@]host:port`, to connect via an HTTP proxy.
- `socks5[h]://[user:pass@]host:port`, to connect via a SOCKS5 or SOCKS5h proxy.

**IMPORTANT**: Netdata does not currently support secure connections to proxies. Data exchanged between Netdata Agents and Netdata Cloud are still end-to-end encrypted, since the Netdata Agent requests a TCP tunnel (HTTP `CONNECT`) from the proxy, and the Netdata Agent directly handles all encryption required for Netdata Cloud communication, however the initial communication from the Netdata Agent to the proxy is not encrypted.

The current implementation uses HTTP proxies in a way that maintains end-to-end encryption between the Netdata agent and Netdata Cloud. Here's how it works:

1. **Proxy Connection**: The agent connects to the HTTP proxy using a plain HTTP connection.
2. **TCP Tunneling Request**: The agent sends an HTTP CONNECT request to the proxy, asking it to establish a TCP tunnel to the Netdata Cloud server.
3. **Proxy Tunneling**: Once the proxy accepts the CONNECT request (responds with HTTP 200), it creates a TCP tunnel between the agent and the Netdata Cloud server. At this point, the proxy simply forwards raw TCP data in both directions without interpreting it.
4. **Encrypted Communication**: The agent then establishes a TLS/SSL connection through this tunnel directly with the Netdata Cloud server. All subsequent data (including the WebSocket handshake and MQTT protocol data) is encrypted end-to-end.

The proxy never sees the decrypted content of the communication - it only sees encrypted TLS traffic flowing through the tunnel it established. This is a standard way of using HTTP proxies for secure connections and is often called "TCP tunneling" or "HTTP CONNECT tunneling."

Keep in mind that there are 2 distinct connection libraries involved. Claiming uses libcurl which may be more flexible, but later at the establishment of the actual Netdata Cloud connection a different library implements MQTT over WebSockets over HTTPS (MQTToWSoHTTPS) and this library does not support encrypted connections to proxies. So, while claiming (libcurl) may work via an encrypted connection to a proxy, the actual Netdata Cloud connection (MQTToWSoHTTPS) will later fail if the proxy connection is encrypted.

The proxy configuration patterns described above, work for both libraries and provide end-to-end encryption for Netdata Cloud communication.

#### Automatically, via environment variables

Netdata will use the following environment variables:

| Option                     | Description                                                                                       | Required |
|----------------------------|---------------------------------------------------------------------------------------------------|----------|
| `NETDATA_CLAIM_URL`        | The Netdata Cloud base URL (defaults to `https://app.netdata.cloud`)                              | no       |
| `NETDATA_CLAIM_TOKEN`      | The claiming token for your Netdata Cloud Space                                                   | yes      |
| `NETDATA_CLAIM_ROOMS`      | A comma-separated list of Rooms that the Agent will be added to                                   | no       |
| `NETDATA_CLAIM_PROXY`      | The URL of a proxy server to use for the connection                                               | no       |
| `NETDATA_EXTRA_CLAIM_OPTS` | May contain a space-separated list of options. The option `-insecure` is the only currently used. | no       |

If the connection process fails, the reason will be logged in daemon.log (search for "CLAIM") and the `cloud` section of `http://ip:19999/api/v3/info`.

## Reconnect

### Linux based installations

To remove a node from your Space in Netdata Cloud, delete the `cloud.d/` directory in your Netdata library directory.

```bash
cd /var/lib/netdata   # Replace with your Netdata library directory, if not /var/lib/netdata/
sudo rm -rf cloud.d/
```

> **IMPORTANT**
>
> Keep in mind that the Agent will be **re-claimed automatically** if the environment variables or `claim.conf` exist when the Agent is restarted.

This node will no longer have access to the credentials it used when connecting to Netdata Cloud via the ACLK.

### Docker based installations

To remove a node from your Space and connect it to another, follow these steps:

1. Enter the running container you wish to remove from your Space

   ```bash
   docker exec -it CONTAINER_NAME sh
   ```

   Replacing `CONTAINER_NAME` with either the container's name or ID.

2. Delete `/var/lib/netdata/cloud.d` and `/var/lib/netdata/registry/netdata.public.unique.id`

   ```bash
   rm -rf /var/lib/netdata/cloud.d/
   rm /var/lib/netdata/registry/netdata.public.unique.id 
   ```

3. Stop and remove the container

   **Docker CLI:**

    ```bash
    docker stop CONTAINER_NAME
    docker rm CONTAINER_NAME
    ```

   Replacing `CONTAINER_NAME` with either the container's name or ID.

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

4. Finally, go to your new Space, copy the installation command with the new claim token and run it.  
   If you’re using a `docker-compose.yml` file, you will have to overwrite it with the new claiming token.  
   The node should now appear online in that Space.

## Regenerate Claiming Token

There may be situations where you need to revoke your previous Claiming Token and generate a new one for security reasons. Here's how to do it:

**Requirements**:

- Only Administrators of a Space in Netdata Cloud can regenerate Claim Tokens.

**Steps**:

1. Navigate to [any screen](#install-and-connect-a-new-agent) containing the Connection command.
2. Click the "Regenerate token" button. This action will invalidate your previous token and generate a new one.

## Troubleshoot

If you're having trouble connecting a node, this may be because the [ACLK](/src/aclk/README.md) cannot connect to Cloud.

With the Netdata Agent running, visit `http://NODE:19999/api/v3/info` in your browser, replacing `NODE` with the IP address or hostname of your Agent. The returned JSON contains a section called `cloud` with helpful information to diagnose any issues you might be having with the ACLK or connection process.

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

### kickstart: unsupported Netdata installation

If you run the kickstart script and get the following error `Existing install appears to be handled manually or through the system package manager.` you most probably installed Netdata using an unsupported package.

Check our [installation section](/packaging/installer/README.md) to find the proper way of installing Netdata on your system.

### kickstart: Failed to write new machine GUID

You might encounter this error if you run the Netdata kickstart script without sufficient permissions:

```bash
Failed to write new machine GUID. Please make sure you have rights to write to /var/lib/netdata/registry/netdata.public.unique.id.
```

To resolve this issue, you have two options:

1. Run the script with root privileges.
2. Run the script with the user that runs the Netdata Agent.

### Connecting to Cloud on older distributions (Ubuntu 14.04, Debian 8, CentOS 6)

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS 6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which helps securely encrypt SSL connections.

We recommend you reinstall Netdata with a [static build](/packaging/installer/methods/kickstart.md#install-type), which uses an up-to-date version of OpenSSL with hostname validation enabled.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to man-in-the-middle attacks.
