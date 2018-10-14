# nfacct.plugin

This plugin that collects NFACCT statistics.

It is currently disabled by default, because it requires root access.
We have to move the code to an external plugin to setuid just the plugin not the whole netdata server.

You can build netdata with it to test it though.
Just run `./configure` (or `netdata-installer.sh`) with the option `--enable-plugin-nfacct` (and any other options you may need).
Remember, you have to tell netdata you want it to run as `root` for this plugin to work.
