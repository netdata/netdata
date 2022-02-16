#!/bin/sh

# Needed because dpkg is stupid and tries to configure things interactively if it sees a terminal.
export DEBIAN_FRONTEND=noninteractive

# Pull in our dependencies
apt update || exit 1
apt upgrade -y || exit 1
apt install -y build-essential debhelper curl gnupg || exit 1

# Run the builds in an isolated source directory.
# This removes the need for cleanup, and ensures anything the build does
# doesn't muck with the user's sources.
cp -a /netdata/packaging/repoconfig /usr/src || exit 1
cd /usr/src/repoconfig || exit 1

# pre/post options are after 1.18.8, is simpler to just check help for their existence than parsing version
if dpkg-buildpackage --help | grep "\-\-post\-clean" 2> /dev/null > /dev/null; then
  dpkg-buildpackage --post-clean --pre-clean -b -us -uc || exit 1
else
  dpkg-buildpackage -b -us -uc || exit 1
fi

# Copy the built packages to /netdata/artifacts (which may be bind-mounted)
# Also ensure /netdata/artifacts exists and create it if it doesn't
[ -d /netdata/artifacts ] || mkdir -p /netdata/artifacts
cp -a /usr/src/*.deb /netdata/artifacts/ || exit 1

# Correct ownership of the artifacts.
# Without this, the artifacts directory and it's contents end up owned
# by root instead of the local user on Linux boxes
chown -R --reference=/netdata /netdata/artifacts
