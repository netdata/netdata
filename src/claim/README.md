# Connect Agent to Cloud

This section guides you through installing and securely connecting a new Netdata Agent to Netdata Cloud via the
encrypted Agent-Cloud Link ([ACLK](/src/aclk/README.md)). Connecting your agent to Netdata Cloud unlocks additional
features like centralized monitoring and easier collaboration.

## Connect

### Install and Connect a New Agent

There are two places in the UI where you can add/connect your Node:

- **Space/Room settings**: Click the cogwheel (the bottom-left corner or next to the Room name at the top) and
  select "Nodes." Click the "+" button to add
  a new node.
- [**Nodes tab**](/docs/dashboards-and-charts/nodes-tab.md): Click on the "Add nodes" button.

Netdata Cloud will generate a command that you can execute on your Node to install and claim the Agent. The command is
available for different installation methods:

| Method              | Description                                                                                                                                                                                                      |
|---------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Linux/FreeBSD/macOS | Install directly using the [kickstart.sh](/packaging/installer/methods/kickstart.md) script.                                                                                                                     |
| Docker              | Install as a container using the provided docker run command or YAML files (Docker Compose/Swarm).                                                                                                               |
| Kubernetes          | Install inside the cluster using `helm`. **Important**: refer to the [Kubernetes installation](/packaging/installer/methods/kubernetes.md#deploy-netdata-on-your-kubernetes-cluster) for detailed  instructions. |

Once you've chosen your installation method, follow the provided instructions to install and connect the Agent.

### Connect an Existing Agent

There are three methods to connect an already installed Netdata Agent to your Netdata Cloud Space:

- Manually via the UI
- Automatically via a provisioning system (or the command line)
- Automatically via environment variables (e.g. kubernetes, docker, etc)

#### Manually, via the UI

The UI method is the easiest and recommended way to connect your Agent. Here's how:

1. Open your Agent local UI.
2. Sign in to your Netdata Cloud account.
3. Click the "Connect" button.
4. Follow the on-screen instructions to connect your Agent.

#### Automatically, via a provisioning system or the command line

Netdata Agents can be connected to Netdata Cloud by creating the file `/etc/netdata/claim.conf`
(or `/opt/netdata/etc/netdata/claim.conf` depending on your installation), like this:

```bash
[global]
   token = The claiming token for your Netdata Cloud Space (required)
   rooms = A comma-separated list of Rooms to add the Agent to (optional)
   proxy = The URL of a proxy server to use for the connection (optional)
   insecure = Either yes or no (optional)
```

example:

```bash
[global]
   token = NETDATA_CLOUD_SPACE_TOKEN
   rooms = ROOM_KEY1,ROOM_KEY2,ROOM_KEY3
   proxy = http://username:password@myproxy:8080
   insecure = no
```

If the agent is already running, you can either run `netdatacli reload-claiming-state` or restart the agent.
Otherwise, the agent will be claimed when it starts.

If claiming fails for whatever reason, daemon.log will log the reason (search for `CLAIM`),
and also `http://ip:19999/api/v2/info` would also state the reason at the `cloud` section of the response.

#### Automatically, via environment variables

Netdata will use the following environment variables:

- `NETDATA_CLAIM_URL`
- `NETDATA_CLAIM_TOKEN`
- `NETDATA_CLAIM_ROOMS`
- `NETDATA_CLAIM_PROXY`
- `NETDATA_EXTRA_CLAIM_OPTS`, may contain a space separated list of options. The option `-insecure` is the only currently used.

The `NETDATA_CLAIM_TOKEN` alone is enough for triggering the claiming process.

If claiming fails for whatever reason, daemon.log will log the reason (search for `CLAIM`),
and also `http://ip:19999/api/v2/info` would also state the reason at the `cloud` section of the response.

## Reconnect

### Linux based installations

To remove a node from your Space in Netdata Cloud, delete the `cloud.d/` directory in your Netdata library directory.

```bash
cd /var/lib/netdata   # Replace with your Netdata library directory, if not /var/lib/netdata/
sudo rm -rf cloud.d/
```

> IMPORTANT:<br/>
> Keep in mind that the Agent will be **re-claimed automatically** if the environment variables or `claim.conf` exist when the agent is restarted. 

This node no longer has access to the credentials it was used when connecting to Netdata Cloud via the ACLK. You will
still be able to see this node in your Rooms in an **unreachable** state.

If you want to reconnect this node, you need to:

1. Ensure that the `/var/lib/netdata/cloud.d` directory doesn't exist. In some installations, the path
   is `/opt/netdata/var/lib/netdata/cloud.d`
2. Stop the Agent
3. Ensure that the `uuidgen-runtime` package is installed. Run ```echo "$(uuidgen)"``` and validate you get back a UUID
4. Copy the kickstart.sh command to add a node from your space and add to the end of it `--claim-id "$(uuidgen)"`. Run
   the command and look for the message `Node was successfully claimed.`
5. Start the Agent

### Docker based installations

To remove a node from you Space in Netdata Cloud, and connect it to another Space, follow these steps:

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
   If you are using a `docker-compose.yml` file, you will have to overwrite it with the new claiming token.  
   The node should now appear online in that Space.

## Regenerate Claiming Token

If in case of some security reason, or other, you need to revoke your previous claiming token and generate a new one you
can achieve that from the Netdata Cloud UI.

On any screen where you see the connect the node to Netdata Cloud command you'll see above it, next to
the [updates channel](/docs/netdata-agent/versions-and-platforms.md), a
button to **Regenerate token**. This action will invalidate your previous token and generate a fresh new one.

Only the administrators of a Space in Netdata Cloud can trigger this action.

## Troubleshoot

If you're having trouble connecting a node, this may be because
the [ACLK](/src/aclk/README.md) cannot connect to Cloud.

With the Netdata Agent running, visit `http://NODE:19999/api/v1/info` in your browser, replacing `NODE` with the IP
address or hostname of your Agent. The returned JSON contains four keys that will be helpful to diagnose any issues you
might be having with the ACLK or connection process.

```
"cloud-enabled"
"cloud-available"
"agent-claimed"
"aclk-available"
```

> **Note**
>
> On Netdata Agent version `1.32` (`netdata -v` to find your version) and newer, `sudo netdatacli aclk-state` can be
> used to get some diagnostic information about ACLK. Sample output:

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

If you run the kickstart script and get the following
error `Existing install appears to be handled manually or through the system package manager.` you most probably
installed Netdata using an unsupported package.

> **Note**
>
> If you are using an unsupported package, such as a third-party `.deb`/`.rpm` package provided by your distribution,
> please remove that package and reinstall using
>
our [recommended kickstart script](/packaging/installer/methods/kickstart.md).

### kickstart: Failed to write new machine GUID

If you run the kickstart script but don't have privileges required for the actions done on the connecting to Netdata
Cloud process you will get the following error:

```bash
Failed to write new machine GUID. Please make sure you have rights to write to /var/lib/netdata/registry/netdata.public.unique.id.
```

For a successful execution you will need to run the script with root privileges or run it with the user that is running
the Agent.

### Connecting on older distributions (Ubuntu 14.04, Debian 8, CentOS 6)

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS
6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old
versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which
helps securely encrypt SSL connections.

We recommend you reinstall Netdata with
a [static build](/packaging/installer/methods/kickstart.md#static-builds),
which uses an up-to-date version of OpenSSL with hostname validation enabled.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit
with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to
man-in-the-middle attacks.

### agent-claimed is false / Claimed: No

You must [connect your node](#connect).

### aclk-available is false / Online: No

If `aclk-available` is `false` and all other keys are `true`, your Agent is having trouble connecting to the Cloud
through the ACLK. Please check your system's firewall.

If your Agent needs to use a proxy to access the internet, you must set up a proxy for connecting.

If you are certain firewall and proxy settings are not the issue, you should consult the Agent's `error.log`
at `/var/log/netdata/error.log` and contact us
by [creating an issue on GitHub](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=ACLK-available-is-false)
with details about your system and relevant output from `error.log`.

## Connecting reference

In the sections below, you can find reference material for the kickstart script, claiming script, connecting via the
Agent's command line tool, and details about the files found in `cloud.d`.

### The `cloud.conf` file

This section defines how and whether your Agent connects to Netdata Cloud using
the [Agent-Cloud link](/src/aclk/README.md)(ACLK).

| setting        | default                     | info                                                                                                                                                 |
|:---------------|:----------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------|
| enabled        | yes                         | Controls whether the ACLK is active. Set to no to prevent the Agent from connecting to Netdata Cloud.                                                |
| cloud base url | <https://app.netdata.cloud> | The URL for the Netdata Cloud web application. Typically, this should not be changed.                                                                |
| proxy          | env                         | Specifies the proxy setting for the ACLK. Options: none (no proxy), env (use environment's proxy), or a URL (e.g., `http://proxy.example.com:1080`). |

### Connection directory

Netdata stores the Agent's connection-related state in the Netdata library directory under `cloud.d`. For a default
installation, this directory exists at `/var/lib/netdata/cloud.d`. The directory and its files should be owned by the
user that runs the Agent, which is typically the `netdata` user.

The `cloud.d/token` file should contain the claiming-token and the `cloud.d/rooms` file should contain the list of War
Rooms you added that node to.

The user can also put the Cloud endpoint's full certificate chain in `cloud.d/cloud_fullchain.pem` so that the Agent
can trust the endpoint if necessary.
