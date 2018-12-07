# netdata contrib

## Building .deb packages

The `contrib/debian/` directory contains basic rules to build a
Debian package.  It has been tested on Debian Jessie and Wheezy,
but should work, possibly with minor changes, if you have other
dpkg-based systems such as Ubuntu or Mint.

To build netdata for a Debian Jessie system, the debian directory
has to be available in the root of the netdata source. The easiest
way to do this is with a symlink:

    ~/netdata$ ln -s contrib/debian

Then build the debian package:

    ~/netdata$ dpkg-buildpackage -us -uc -rfakeroot

This should give a package that can be installed in the parent
directory, which you can install manually with dpkg.

    ~/netdata$ ls ../*.deb
    ../netdata_1.0.0_amd64.deb
    ~/netdata$ sudo dpkg -i ../netdata_1.0.0_amd64.deb


### Building for a Debian system without systemd

The included packaging is designed for modern Debian systems that
are based on systemd. To build non-systemd packages (for example,
for Debian wheezy), you will need to make a couple of minor
updates first.

* edit `contrib/debian/rules` and adjust the `dh` rule near the
  top to remove systemd (see comments in that file).

* rename `contrib/debian/control.wheezy` to `contrib/debian/control`.

* change `control.wheezy from contrib/Makefile* to control`.

* uncomment `EXTRA_OPTS="-P /var/run/netdata.pid"` in
 `contrib/debian/netdata.default`

* edit `contrib/debian/netdata.init` and change `PIDFILE` to
  `/var/run/netdata.pid`

* remove `dpkg-statoverride --update --add --force root netdata 0775 /var/lib/netdata/registry` from
  `contrib/debian/netdata.postinst.in`. If you are going to handle the unique id file differently.

Then proceed as the main instructions above.

### Reinstalling netdata

The recommended way to upgrade netdata packages built from this
source is to remove the current package from your system, then
install the new package. Upgrading on wheezy is known to not
work cleanly; Jessie may behave as expected.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcontrib%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
