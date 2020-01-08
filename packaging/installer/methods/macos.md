# Install Netdata on macOS

Netdata on macOS still has limited charts, but external plugins do work.

## With Homebrew

You can either install Netdata with [Homebrew](https://brew.sh/)

```sh
brew install netdata
```

## From source

or from source:

```sh
# install Xcode Command Line Tools
xcode-select --install
```

click `Install` in the software update popup window, then

```sh
# install HomeBrew package manager
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"

# install required packages
brew install ossp-uuid autoconf automake pkg-config

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start Netdata when your Mac boots.
