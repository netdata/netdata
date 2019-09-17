# node.d.plugin

`node.d.plugin` is a Netdata external plugin. It is an **orchestrator** for data collection modules written in `node.js`.

1.  It runs as an independent process `ps fax` shows it
2.  It is started and stopped automatically by Netdata
3.  It communicates with Netdata via a unidirectional pipe (sending data to the `netdata` daemon)
4.  Supports any number of data collection **modules**
5.  Allows each **module** to have one or more data collection **jobs**
6.  Each **job** is collecting one or more metrics from a single data source

## Pull Request Checklist for Node.js Plugins

This is a generic checklist for submitting a new Node.js plugin for Netdata.  It is by no means comprehensive.

At minimum, to be buildable and testable, the PR needs to include:

-   The module itself, following proper naming conventions: `node.d/<module_dir>/<module_name>.node.js`
-   A README.md file for the plugin. 
-   The configuration file for the module
-   A basic configuration for the plugin in the appropriate global config file: `conf.d/node.d.conf`, which is also in JSON format.  If the module should be enabled by default, add a section for it in the `modules` dictionary.
-   A line for the plugin in the appropriate `Makefile.am` file: `node.d/Makefile.am` under `dist_node_DATA`.
-   A line for the plugin configuration file in `conf.d/Makefile.am`: under `dist_nodeconfig_DATA`
-   Optionally, chart information in `web/dashboard_info.js`.  This generally involves specifying a name and icon for the section, and may include descriptions for the section or individual charts.

## Motivation

Node.js is perfect for asynchronous operations. It is very fast and quite common (actually the whole web is based on it).
Since data collection is not a CPU intensive task, node.js is an ideal solution for it.

`node.d.plugin` is a Netdata plugin that provides an abstraction layer to allow easy and quick development of data
collectors in node.js. It also manages all its data collectors (placed in `/usr/libexec/netdata/node.d`) using a single
instance of node, thus lowering the memory footprint of data collection.

Of course, there can be independent plugins written in node.js (placed in `/usr/libexec/netdata/plugins`).
These will have to be developed using the guidelines of **[External Plugins](../plugins.d/)**.

To run `node.js` plugins you need to have `node` installed in your system.

In some older systems, the package named `node` is not node.js. It is a terminal emulation program called `ax25-node`.
In this case the node.js package may be referred as `nodejs`. Once you install `nodejs`, we suggest to link
`/usr/bin/nodejs` to `/usr/bin/node`, so that typing `node` in your terminal, opens node.js.
For more information check the **\[[Installation]]** guide.

## configuring `node.d.plugin`

`node.d.plugin` can work even without any configuration. Its default configuration file is
[/etc/netdata/node.d.conf](node.d.conf) (to edit it on your system run `/etc/netdata/edit-config node.d.conf`).

## configuring `node.d.plugin` modules

`node.d.plugin` modules accept configuration in `JSON` format.

Unfortunately, `JSON` files do not accept comments. So, the best way to describe them is to have markdown text files
with instructions.

`JSON` has a very strict formatting. If you get errors from Netdata at `/var/log/netdata/error.log` that a certain
configuration file cannot be loaded, we suggest to verify it at <http://jsonlint.com/>.

The files in this directory, provide usable examples for configuring each `node.d.plugin` module.

## debugging modules written for node.d.plugin

To test `node.d.plugin` modules, which are placed in `/usr/libexec/netdata/node.d`, you can run `node.d.plugin` by hand,
like this:

```sh
# become user netdata
sudo su -s /bin/sh netdata

# run the plugin in debug mode
/usr/libexec/netdata/plugins.d/node.d.plugin debug 1 X Y Z
```

`node.d.plugin` will run in `debug` mode (lots of debug info), with an update frequency of `1` second, evaluating only
the collector scripts `X` (i.e. `/usr/libexec/netdata/node.d/X.node.js`), `Y` and `Z`.
You can define zero or more modules. If none is defined, `node.d.plugin` will evaluate all modules available.

Keep in mind that if your configs are not in `/etc/netdata`, you should do the following before running `node.d.plugin`:

```sh
export NETDATA_USER_CONFIG_DIR="/path/to/etc/netdata"
```

---

## developing `node.d.plugin` modules

Your data collection module should be split in 3 parts:

-   a function to fetch the data from its source. `node.d.plugin` already can fetch data from web sources,
     so you don't need to do anything about it for http.

-   a function to process the fetched/manipulate the data fetched. This function will make a number of calls
     to create charts and dimensions and pass the collected values to Netdata.
     This is the only function you need to write for collecting http JSON data.

-   a `configure` and an `update` function, which take care of your module configuration and data refresh
     respectively. You can use the supplied ones.

Your module will automatically be able to process any number of servers, with different settings (even different
data collection frequencies). You will write just the work needed for one and `node.d.plugin` will do the rest.
For each server you are going to fetch data from, you will have to create a `service` (more later).

### writing the data collection module

To provide a module called `mymodule`, you have create the file `/usr/libexec/netdata/node.d/mymodule.node.js`, with this structure:

```js
// the processor is needed only
// if you need a custom processor
// other than http
netdata.processors.myprocessor = {
	name: 'myprocessor',

	process: function(service, callback) {

		/* do data collection here */

		callback(data);
	}
};

// this is the mymodule definition
var mymodule = {
	processResponse: function(service, data) {

		/* send information to the Netdata server here */

	},

	configure: function(config) {
		var eligible_services = 0;

		if(typeof(config.servers) === 'undefined' || config.servers.length === 0) {

			/*
			 * create a service using internal defaults;
			 * this is used for auto-detecting the settings
			 * if possible
			 */

			netdata.service({
				name: 'a name for this service',
				update_every: this.update_every,
				module: this,
				processor: netdata.processors.myprocessor,
				// any other information your processor needs
			}).execute(this.processResponse);

			eligible_services++;
		}
		else {

			/*
			 * create a service for each server in the
			 * configuration file
			 */

			var len = config.servers.length;
			while(len--) {
				var server = config.servers[len];

				netdata.service({
					name: server.name,
					update_every: server.update_every,
					module: this,
					processor: netdata.processors.myprocessor,
					// any other information your processor needs
				}).execute(this.processResponse);

				eligible_services++;
			}
		}

		return eligible_services;
	},

	update: function(service, callback) {
		
		/*
		 * this function is called when each service
		 * created by the configure function, needs to
		 * collect updated values.
		 *
		 * You normally will not need to change it.
		 */

		service.execute(function(service, data) {
			mymodule.processResponse(service, data);
			callback();
		});
	},
};

module.exports = mymodule;
```

#### configure(config)

`configure(config)` is called just once, when `node.d.plugin` starts.
The config file will contain the contents of `/etc/netdata/node.d/mymodule.conf`.
This file should have the following format:

```js
{
	"enable_autodetect": false,
	"update_every": 5,
	"servers": [ { /* server 1 */ }, { /* server 2 */ } ]
}
```

If the config file `/etc/netdata/node.d/mymodule.conf` does not give a `enable_autodetect` or `update_every`, these
will be added by `node.d.plugin`. So you module will always have them.

The configuration file `/etc/netdata/node.d/mymodule.conf` may contain whatever else is needed for `mymodule`.

#### processResponse(data)

`data` may be `null` or whatever the processor specified in the `service` returned.

The `service` object defines a set of functions to allow you send information to the Netdata core about:

1.  Charts and dimension definitions
2.  Updated values, from the collected values

---

_FIXME: document an operational node.d.plugin data collector - the best example is the
[snmp collector](snmp/snmp.node.js)_

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnode.d.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
