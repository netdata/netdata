# Connect Agent to Cloud

This section guides you through installing and securely connecting a new Agent to Netdata Cloud via the encrypted Agent-Cloud Link ([ACLK](/src/aclk/README.md)). Connecting your Agent to Netdata Cloud unlocks additional features like centralized monitoring and easier collaboration.

## Connect

### Install and Connect a New Agent

There are two places in the UI where you can add/connect your Node:

- **Space/Room settings**: Click the cogwheel (the bottom-left corner or next to the Room name at the top) and
  select "Nodes." Click the "+" button to add a new node.
- [**Nodes tab**](/docs/dashboards-and-charts/nodes-tab.md): Click on the "Add nodes" button.

Netdata Cloud will generate a command that you can execute on your Node to install and connect the Agent to your Space. The command is available for different installation methods:

| Method              | Description                                                                                                                                                                                                      |
|---------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Linux/FreeBSD/macOS | Install directly using the [kickstart.sh](/packaging/installer/methods/kickstart.md) script.                                                                                                                     |
| Docker              | Install as a container using the provided docker run command or YAML files (Docker Compose/Swarm).                                                                                                               |
| Kubernetes          | Install inside the cluster using `helm`. **Important**: refer to the [Kubernetes installation](/packaging/installer/methods/kubernetes.md#deploy-netdata-on-your-kubernetes-cluster) for detailed  instructions. |

Once you've chosen your installation method, follow the provided instructions to install and connect the Agent.

### Connect an Existing Agent

There are three methods to connect an already installed Netdata Agent to your Netdata Cloud Space:

- Manually, via the UI
- Automatically, via a provisioning system (or the command line)
- Automatically, via environment variables (e.g. kubernetes, docker, etc)

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
   url = The Netdata Cloud base URL (optional, defaults to `https://app.netdata.cloud`)
   token = The claiming token for your Netdata Cloud Space (required)
   rooms = A comma-separated list of Rooms to add the Agent to (optional)
   proxy = The URL of a proxy server to use for the connection, or none, or env (optional, defaults to env)
   insecure = Either yes or no (optional)
```

- `proxy` can get anything libcurl accepts as a proxy, or the `none` and `env` keywords. `none` (or just an empty value) disables proxy configuration, while `env` tells libcurl to use the environment to determine the proxy configuration (usually the `https_proxy` environment variable).
- `insecure` is a boolean (either `yes`, or `no`) and when set to `yes` it instructs libcurl to disable host verification.

example:

```bash
[global]
   url = https://app.netdata.cloud
   token = NETDATA_CLOUD_SPACE_TOKEN
   rooms = ROOM_KEY1,ROOM_KEY2,ROOM_KEY3
   proxy = http://username:password@myproxy:8080
   insecure = no
```

If the Agent is already running, you can either run `netdatacli reload-claiming-state` or restart the Agent. Otherwise, the Agent will be connected when it starts.

If the connection process fails, the reason will be logged in daemon.log (search for "CLAIM") and the `cloud` section of `http://ip:19999/api/v2/info`.

#### Automatically, via environment variables

Netdata will use the following environment variables:

- `NETDATA_CLAIM_URL`: The Netdata Cloud base URL (optional, defaults to `https://app.netdata.cloud`)
- `NETDATA_CLAIM_TOKEN`: The claiming token for your Netdata Cloud Space (required)
- `NETDATA_CLAIM_ROOMS`: A comma-separated list of Rooms to add the Agent to (optional)
- `NETDATA_CLAIM_PROXY`: The URL of a proxy server to use for the connection (optional)
- `NETDATA_EXTRA_CLAIM_OPTS`, may contain a space separated list of options. The option `-insecure` is the only currently used.

The `NETDATA_CLAIM_TOKEN` alone is enough for triggering the connection process.

If the connection process fails, the reason will be logged in daemon.log (search for "CLAIM") and the `cloud` section of `http://ip:19999/api/v2/info`.

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

This node no longer has access to the credentials it was used when connecting to Netdata Cloud via the ACLK. You will
still be able to see this node in your Rooms in an **unreachable** state.

### Docker based installations

To remove a node from your Space in Netdata Cloud and connect it to another Space, follow these steps:

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

There may be situations where you need to revoke your previous Netdata Cloud claiming token and generate a new one for security reasons. Here's how to do it:

**Requirements**:

- Only administrators of Space in Netdata Cloud can regenerate tokens.

**Steps**:

1. Navigate to any screen within the Netdata Cloud UI where you see the "Connect the node to Netdata Cloud" command.
2. Look above this command, near the [Updates channel](/docs/netdata-agent/versions-and-platforms.md). You should see a button that says "Regenerate token."
3. Click the "Regenerate token" button. This action will invalidate your previous token and generate a new one.

## Troubleshoot

If you're having trouble connecting a node, this may be because
the [ACLK](/src/aclk/README.md) cannot connect to Cloud.

With the Netdata Agent running, visit `http://NODE:19999/api/v2/info` in your browser, replacing `NODE` with the IP
address or hostname of your Agent. The returned JSON contains a section called `cloud` with helpful information to
diagnose any issues you might be having with the ACLK or connection process.

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
> If you’re using an unsupported package, such as a third-party `.deb`/`.rpm` package provided by your distribution,
> please remove that package and reinstall using
>
our [recommended kickstart script](/packaging/installer/methods/kickstart.md).

### kickstart: Failed to write new machine GUID

You might encounter this error if you run the Netdata kickstart script without sufficient permissions:

```bash
Failed to write new machine GUID. Please make sure you have rights to write to /var/lib/netdata/registry/netdata.public.unique.id.
```

To resolve this issue, you have two options:

1. Run the script with root privileges.
2. Run the script with the user that runs the Netdata Agent.

### Connecting to Cloud on older distributions (Ubuntu 14.04, Debian 8, CentOS 6)

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS
6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old
versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation),
which helps securely encrypt SSL connections.

We recommend you reinstall Netdata with a [static build](/packaging/installer/methods/kickstart.md#install-type), which uses an up-to-date version of OpenSSL with hostname validation enabled.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit
with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to
man-in-the-middle attacks.
