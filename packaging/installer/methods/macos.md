# Install Netdata on macOS

You can install Netdata in one of the three following ways:

- **[Install Netdata with the automatic one-line installation script (recommended)](#install-netdata-with-our-automatic-one-line-installation-script)**,
- [Install Netdata via Homebrew](#install-netdata-via-homebrew)
- [Install Netdata from source](#install-netdata-from-source)

Each of these installation option requires [Homebrew](https://brew.sh/) for handling dependencies.

:::info

The Netdata Homebrew package is community-created and -maintained.

:::

:::note

Community-maintained packages _may_ receive support from Netdata, but are only a best-effort affair. Learn more about [Netdata's platform support policy](/packaging/PLATFORM_SUPPORT.md).

:::

## Install Netdata with our automatic one-line installation script

### Local Netdata Agent installation

To install Netdata using our automatic [kickstart](/packaging/installer/methods/kickstart.md) open a new terminal and run:

```bash
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh
```

The Netdata Agent is installed under `/usr/local/netdata`. Dependencies are handled via Homebrew.

### Automatically connect to Netdata Cloud during installation

The `kickstart.sh` script accepts additional parameters to automatically [connect](/src/claim/README.md) your node to Netdata
Cloud immediately after installation. Find the `token` and `rooms` strings by [signing in to Netdata
Cloud](https://app.netdata.cloud/sign-in?cloudRoute=/spaces), then clicking on **Connect Nodes** on any of the prompts from the UI.

- `--claim-token`: Specify a unique claiming token associated with your Space in Netdata Cloud to be used to connect to the node
  after the install.
- `--claim-rooms`: Specify a comma-separated list of tokens for each Room this node should appear in.
- `--claim-proxy`: Specify a proxy to use when connecting to the Cloud in the form of `http://[user:pass@]host:ip` for an HTTP(S) proxy.
  See [connecting through a proxy](/src/claim/README.md#proxy-configuration) for details.
- `--claim-url`: Specify a URL to use when connecting to the Cloud. Defaults to `https://app.netdata.cloud`.

For example:

```bash
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --install-prefix /usr/local/ --claim-token TOKEN --claim-rooms ROOM1,ROOM2 --claim-url https://app.netdata.cloud
```

The Netdata Agent is installed under `/usr/local/netdata` on your machine. Your machine will also show up as a node in your Netdata Cloud.

If you experience issues while connecting your node, follow the steps in our [Troubleshoot](/src/claim/README.md#troubleshooting) documentation.

## Install Netdata via Homebrew

### For macOS Intel

To install Netdata and all its dependencies, run Homebrew using the following command:

```sh
brew install netdata
```

Homebrew will place your Netdata configuration directory at `/usr/local/etc/netdata/`.

Use the `edit-config` script and the files in this directory to configure Netdata. For reference, you can find stock configuration files at `/usr/local/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

To connect this Agent to Netdata Cloud, see [Connect a Homebrew-installed Agent to Netdata Cloud](#connect-a-homebrew-installed-agent-to-netdata-cloud) below.

### For Apple Silicon

To install Netdata and all its dependencies, run Homebrew using the following command:

```sh
brew install netdata
```

Homebrew will place your Netdata configuration directory at `/opt/homebrew/etc/netdata/`.

Use the `edit-config` script and the files in this directory to configure Netdata. For reference, you can find stock configuration files at `/opt/homebrew/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

To connect this Agent to Netdata Cloud, see [Connect a Homebrew-installed Agent to Netdata Cloud](#connect-a-homebrew-installed-agent-to-netdata-cloud) below.

### Connect a Homebrew-installed Agent to Netdata Cloud

The easiest way to connect a Homebrew-installed Netdata Agent to Netdata Cloud is via the local dashboard UI, as described in [Method 1: Via UI](/src/claim/README.md#method-1-via-ui-recommended):

1. Open the local dashboard in your browser at `http://localhost:19999` (or the Agent's IP address at port 19999).
2. Sign in to your Netdata Cloud account.
3. Click the **Connect** button and follow the on-screen instructions.

For automated setups or headless machines where the UI is not accessible, you can use one of these alternatives:

- **Kickstart script claiming flags** — this requires installing/reinstalling with kickstart (it installs under `/usr/local/netdata`) rather than adding flags to an existing Homebrew install. If you want to keep the Homebrew install, use the **Configuration file** method below. See the [kickstart claiming section](#automatically-connect-to-netdata-cloud-during-installation) above or the full [kickstart documentation](/packaging/installer/methods/kickstart.md).
- **Configuration file** — create a `claim.conf` file in your Netdata configuration directory using the [configuration file method](/src/claim/README.md#method-2-via-configuration-file).

**Configuration directory paths for `claim.conf`:**

| Architecture | Path |
|:--|:--|
| Intel | `/usr/local/etc/netdata/claim.conf` |
| Apple Silicon | `/opt/homebrew/etc/netdata/claim.conf` |

:::note

On macOS, Homebrew installs run under your user account and the `netdata` group does not exist. Use your own user and the `staff` group for file ownership when creating `claim.conf` manually. For full details on permissions and applying the configuration, see the [configuration file method](/src/claim/README.md#method-2-via-configuration-file).

:::

:::caution

Do **not** run the `netdata-claim.sh` script manually. It is deprecated and will be unsupported in the near future. Instead, use one of the supported claiming methods described above: the Cloud UI, kickstart claiming flags during install/reinstall, or a `claim.conf` file.

:::

For the full list of claiming options and troubleshooting, see [Connect Agent to Cloud](/src/claim/README.md).

## Install Netdata from source

We don't recommend installing Netdata from source on macOS, as it can be difficult to configure and install dependencies manually.

1. Open your terminal of choice and install the Xcode development packages:

   ```bash
   xcode-select --install
   ```

2. Click **Install** on the Software Update popup window that appears.
3. Use the same terminal session to install some of Netdata's prerequisites using Homebrew. If you don't want to use [Netdata Cloud](/docs/netdata-cloud/README.md), you can omit `cmake`.

   ```bash
   brew install ossp-uuid autoconf automake pkg-config libuv lz4 json-c openssl libtool cmake
   ```

4. Download Netdata from our GitHub repository:

   ```bash
   git clone https://github.com/netdata/netdata.git --recursive
   ```

5. `cd` into the newly-created directory and then start the installer script:

   ```bash
   cd netdata/
   sudo ./netdata-installer.sh --install-prefix /usr/local
   ```

:::info

- Your Netdata configuration directory will be at `/usr/local/netdata/`.
- Your stock configuration directory will be at `/usr/local/lib/netdata/conf.d/`.
- The installer will also install a startup plist to start Netdata when your macOS system boots.

:::

Netdata works on macOS, albeit with some limitations.

- The number of charts displaying system metrics is limited, but you can use any of Netdata's [external plugins](/src/plugins.d/README.md) to monitor any services you might have installed on your macOS system.
- You could also use a macOS system as the parent node in a [streaming configuration](/src/streaming/README.md).
