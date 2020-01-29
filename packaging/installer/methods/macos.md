# Install Netdata on macOS

Netdata works on macOS, albeit with some limitations. The number of charts displaying system metrics is limited, but you
can use any of Netdata's [external plugins](../../../collectors/plugins.d/README.md) to monitor any services you might
have installed on your macOS system. You could also use a macOS system as the master node in a [streaming
configuration](../../../streaming/README.md).

We support two methods of installing Netdata on macOS, although both involve [Homebrew](https://brew.sh/). Install that
first, then proceed to either install Netdata via a Homebrew package, or directly from source.

The Homebrew package will be easier to install, although it only updates with [major
releases](../README.md#nightly-vs-stable-releases), not nightly updates. 

-   [Install Homebrew on macOS](#install-homebrew-on-macos)
-   [Install Netdata via the Homebrew package](#install-netdata-with-the-homebrew-package)
-   [Install Netdata from source](#install-netdata-from-source)

## Install Homebrew on macOS

The first step, for either method, is to install Homebrew on your macOS system. Use their installation script:

```bash
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
```

Now that you have Homebrew installed, you can move on to either of the two following installation methods.

## Install Netdata with the Homebrew package

Homebrew's community maintains a package that helps you install Netdata and its dependencies in one step:

```sh
brew install netdata
```

> Your Netdata configuration directory will be at `/usr/local/etc/netdata/`, and your stock configuration directory will
> be at `/usr/local/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.

## Install Netdata from source

To install Netdata from source, first open your terminal of choice and install Xcode development packages.

```bash
xcode-select --install
```

Click **Install** on the Software Update popup window that appears. Then, use the same terminal session to Homebrew to
install Netdata's prerequisites.

```bash
brew install ossp-uuid autoconf automake pkg-config
```

Next, download Netdata from our GitHub repository:

```bash
git clone https://github.com/netdata/netdata.git --depth=100
```

Finally, `cd` into the newly-created directory and then start the installer script:

```bash
cd netdata/
sudo ./netdata-installer.sh --install /usr/local
```

> Your Netdata configuration directory will be at `/usr/local/etc/netdata/`, and your stock configuration directory will
> be at `/usr/local/Cellar/netdata/{NETDATA_VERSION}/lib/netdata/conf.d/`.
>
> The installer will also install a startup plist to start Netdata when your Mac boots.

## What's next?

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs.

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../../docs/getting-started.md) for a quick overview.
