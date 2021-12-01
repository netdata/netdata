<!--
title: "Netdata contrib"
custom_edit_url: https://github.com/netdata/netdata/edit/master/contrib/README.md
-->

# Netdata contrib

## Building .deb packages

The `contrib/debian/` directory contains basic rules to build a
Debian package.  It has been tested on Debian Jessie and Wheezy,
but should work, possibly with minor changes, if you have other
dpkg-based systems such as Ubuntu or Mint.

To build Netdata for a Debian Jessie system, the debian directory
has to be available in the root of the Netdata source. The easiest
way to do this is with a symlink:

```sh
ln -s contrib/debian
```

Edit the `debian/changelog` file to reflect the package version and
the build time:

```sh
netdata (1.21.0) unstable; urgency=medium

  * Initial Release

 -- Netdata Builder <bot@netdata.cloud>   Tue, 12 May 2020 10:36:52 +0200
```

Then build the debian package:

```sh
dpkg-buildpackage -us -uc -rfakeroot
```

This should give a package that can be installed in the parent
directory, which you can install manually with dpkg.

```sh
ls -1 ../*.deb
../netdata_1.21.0_amd64.deb
../netdata-dbgsym_1.21.0_amd64.deb
../netdata-plugin-cups_1.21.0_amd64.deb
../netdata-plugin-cups-dbgsym_1.21.0_amd64.deb
../netdata-plugin-freeipmi_1.21.0_amd64.deb
../netdata-plugin-freeipmi-dbgsym_1.21.0_amd64.deb
sudo dpkg -i ../netdata_1.21.0_amd64.deb
```

### Reinstalling Netdata

The recommended way to upgrade Netdata packages built from this
source is to remove the current package from your system, then
install the new package. Upgrading on wheezy is known to not
work cleanly; Jessie may behave as expected.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcontrib%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
