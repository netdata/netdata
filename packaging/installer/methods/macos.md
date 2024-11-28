# Install Netdata on macOS

Netdata works on macOS, albeit with some limitations.
The number of charts displaying system metrics is limited, but you can use any of Netdata's [external plugins](/src/plugins.d/README.md) to monitor any services you might have installed on your macOS system.
You could also use a macOS system as the parent node in a [streaming configuration](/src/streaming/README.md).

You can install Netdata in one of the three following ways:

- **[Install Netdata with the our automatic one-line installation script (recommended)](#install-netdata-with-our-automatic-one-line-installation-script)**,
- [Install Netdata via Homebrew](#install-netdata-via-homebrew)
- [Install Netdata from source](#install-netdata-from-source)

Each of these installation option requires [Homebrew](https://brew.sh/) for handling dependencies.

> The Netdata Homebrew package is community-created and -maintained.
> Community-maintained packages _may_ receive support from Netdata, but are only a best-effort affair. Learn more about [Netdata's platform support policy](/docs/netdata-agent/versions-and-platforms.md).

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
  See [connecting through a proxy](/src/claim/README.md#automatically-via-a-provisioning-system-or-the-command-line) for details.
- `--claim-url`: Specify a URL to use when connecting to the Cloud. Defaults to `https://app.netdata.cloud`.

For example:

```bash
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --install-prefix /usr/local/ --claim-token TOKEN --claim-rooms ROOM1,ROOM2 --claim-url https://app.netdata.cloud
```

The Netdata Agent is installed under `/usr/local/netdata` on your machine. Your machine will also show up as a node in your Netdata Cloud.

If you experience issues while connecting your node, follow the steps in our [Troubleshoot](/src/claim/README.md#troubleshoot) documentation.

## Install Netdata via Homebrew

### For macOS Intel

To install Netdata and all its dependencies, run Homebrew using the following command:

```sh
brew install netdata
```

Homebrew will place your Netdata configuration directory at `/usr/local/etc/netdata/`.

Use the `edit-config` script and the files in this directory to configure Netdata. For reference, you can find stock configuration files at `/usr/local/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

### For Apple Silicon

To install Netdata and all its dependencies, run Homebrew using the following command:

```sh
brew install netdata
```

Homebrew will place your Netdata configuration directory at `/opt/homebrew/etc/netdata/`.

Use the `edit-config` script and the files in this directory to configure Netdata. For reference, you can find stock configuration files at `/opt/homebrew/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

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

> Your Netdata configuration directory will be at `/usr/local/netdata/`.
> Your stock configuration directory will be at `/usr/local/lib/netdata/conf.d/`.
> The installer will also install a startup plist to start Netdata when your macOS system boots.
