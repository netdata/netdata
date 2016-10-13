`node.d.plugin` modules accept configuration in JSON format.

Unfortunately, JSON files do not accept comments. So, the best way to describe them is to have markdown text files with instructions.

JSON has a very strict formatting. If you get errors from netdata at `/var/log/netdata/error.log` that a certain configuration file cannot be loaded, we suggest to verify it at [http://jsonlint.com/](http://jsonlint.com/).

The files in this directory, provide usable examples for configuring each `node.d.plugin` module.
