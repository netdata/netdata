# nfacct.plugin

This plugin that collects NFACCT statistics.

It is currently disabled by default, because it requires root access.
We have to move the code to an external plugin to setuid just the plugin not the whole netdata server.

You can build netdata with it to test it though.
Just run `./configure` (or `netdata-installer.sh`) with the option `--enable-plugin-nfacct` (and any other options you may need).
Remember, you have to tell netdata you want it to run as `root` for this plugin to work.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnfacct.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
