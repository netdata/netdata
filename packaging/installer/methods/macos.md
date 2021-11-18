<!--
title: "Install Netdata on macOS"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/macos.md
-->

# Install Netdata on macOS

Netdata works on macOS, albeit with some limitations. The number of charts displaying system metrics is limited, but you
can use any of Netdata's [external plugins](/collectors/plugins.d/README.md) to monitor any services you might
have installed on your macOS system. You could also use a macOS system as the parent node in a [streaming
configuration](/streaming/README.md).

We recommend you to **[install Netdata with the our automatic one-line installation script](#install-netdata-with-our-automatic-one-line-installation-script)**, 


As an alternative you also have community-created and -maintained [**Homebrew
package**](#install-netdata-with-the-homebrew-package). 

- [Install Netdata via the Homebrew package](#install-netdata-with-the-homebrew-package)
- [Install Netdata from source](#install-netdata-from-source)

Being community-created and -maintained we don't guarantee that the features made available on our installation script will also be available or give support to it.

## Install Netdata with our automatic one-line installation script

To install Netdata using our automatic [kickstart](/packaging/installer/README.md#automatic-one-line-installation-script) script you will just need to run:

```bash
rm -f ./kickstart.sh ; wget https://my-netdata.io/kickstart.sh && sh ./kickstart.sh
```

With this script, you are also able to connect your nodes directly to Netdata Cloud if you wish, see more details on [Connect an agent running in macOS](/claim/README.md#connect-an-agent-running-in-macos)

This currently only supports building Netdata locally, and requires dependencies to be handled either via Homebrew
or MacPorts (we preferentially use Homebrew if both are found). By default, this will install Netdata under
`/usr/local/netdata`.

## Install Netdata with the Homebrew package

If you don't have [Homebrew](https://brew.sh/) installed already, begin with their installation script:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install.sh)"
```

Next, you can use Homebrew's package, which installs Netdata all its dependencies in a single step:

```sh
brew install netdata
```

> Homebrew will place your Netdata configuration directory at `/usr/local/etc/netdata/`. Use the `edit-config` script
> and the files in this directory to configure Netdata. For reference, you can find stock configuration files at
> `/usr/local/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

Skip on ahead to the [What's next?](#whats-next) section to find links to helpful post-installation guides.

## Install Netdata from source

We don't recommend installing Netdata from source on macOS, as it can be difficult to configure and install dependencies
manually.

First open your terminal of choice and install the Xcode development packages.

```bash
xcode-select --install
```

Click **Install** on the Software Update popup window that appears. Then, use the same terminal session to use Homebrew
to install some of Netdata's prerequisites. You can omit `cmake` in case you do not want to use 
[Netdata Cloud](https://learn.netdata.cloud/docs/cloud/).

```bash
brew install ossp-uuid autoconf automake pkg-config libuv lz4 json-c openssl@1.1 libtool cmake
```

If you want to use the [database engine](/database/engine/README.md) to store your metrics, you need to download
and install the [Judy library](https://sourceforge.net/projects/judy/) before proceeding compiling Netdata.

Next, download Netdata from our GitHub repository:

```bash
git clone https://github.com/netdata/netdata.git --recursive
```

Finally, `cd` into the newly-created directory and then start the installer script:

```bash
cd netdata/
sudo ./netdata-installer.sh --install /usr/local
```

> Your Netdata configuration directory will be at `/usr/local/netdata/`, and your stock configuration directory will
> be at **`/usr/local/lib/netdata/conf.d/`.**
>
> The installer will also install a startup plist to start Netdata when your macOS system boots.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fmacos&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
