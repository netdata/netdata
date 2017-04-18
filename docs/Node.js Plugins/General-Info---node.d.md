# node.js plugins

Node.js is perfect for asynchronous operations, and since data collection should not be a CPU intensive task, using node.js for data collection provides an ideal solution.

`node.d.plugin` is a netdata plugin that provides an abstraction layer to allow easy and quick development of data collectors in node.js. It also manages all its data collectors (placed in `/usr/libexec/netdata/node.d`) using a single instance of node, thus lowering the memory footprint of data collection.

Of course, there can be independent plugins written in node.js (placed in `/usr/libexec/netdata/plugins`). These will have to be developed using the guidelines of **[[External Plugins]]**.

To run `node.js` plugins you need to have `node` installed in your system.

In some older systems, the package named `node` is not node.js. It is a terminal emulation program called `ax25-node`. In this case the node.js package may be referred as `nodejs`. Once you install `nodejs`, you will have to link `/usr/bin/nodejs` to `/usr/bin/node` for this plugin to work, so that typing `node` in your terminal, opens node.js. For more information check the **[[Installation]]** guide.

## configuring `node.d.plugin`

`node.d.plugin` can work even without any configuration, if it can find an operational collector in `/usr/libexec/netdata/node.d`.

Using its configuration file `/etc/netdata/node.d.conf` you can control its default behaviour

*FIXME: document the options and the file format*

## debugging collectors written for node.d.plugin

To test `node.d.plugin` collectors, which are placed in `/usr/libexec/netdata/node.d`:

You can run `node.d.plugin` by hand, with something like this:

```sh
/usr/libexec/netdata/plugins.d/node.d.plugin debug 1 X Y Z
```

`node.d.plugin` will run in `debug` mode (lots of debug info), with an update frequency of `1` second, evaluating only the collector scripts `X` (i.e. `/usr/libexec/netdata/node.d/X.node.js`), `Y` and `Z`. You can define zero or more collector scripts. If none is defined, `node.d.plugin` will evaluate all collector script available.

Keep in mind that if your configs are not in `/etc/netdata`, you should do the following before running `node.d.plugin`:

```sh
export NETDATA_CONFIG_DIR="/path/to/etc/netdata"
```

---

## developing `node.d.plugin` collectors

These are the entities involved:

1. `node.d.plugin`

  `node.d.plugin` is a netdata plugin that provides an abstraction layer to allow you write netdata plugins in node.js quickly. It also manages all node.js plugins using a single instance of node, thus lowering the memory footprint of data collection.

2. Your data collection module should be split in 2 parts:

   - a `processor` to fetch the data from its source. `node.d.plugin` already provides an `http` collector to fetch data from web sources.
   - the chart generation, that will instruct netdata what charts to create and which values to update

3. Your module will automatically be able to process any number of servers, with different settings (even different data collection frequencies). You will write just the work needed for one, `node.d.plugin` will do the rest. For each server you are going to fetch data from, you will have to create a `service` (more later).

### writing a collector

To provide a module called `mymodule`, you have create the file `/usr/libexec/netdata/node.d/mymodule.node.js`, with this structure:

```js

// this processor is needed only
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

		/* send information to the netdata server here */

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
The config file will contain the contents of `/etc/netdata/mymodule.conf`.
This file should have the following format:

```js
{
	"enable_autodetect": false,
	"update_every": 5,
	"servers": [ { /* server 1 */ }, { /* server 2 */ } ]
}
```

If the config file `/etc/netdata/mymodule.conf` does not give a `enable_autodetect` or `update_every`, these will be added by `node.d.plugin`. So you module will always have them.

The configuration file `/etc/netdata/mymodule.conf` may contain whatever else is needed for `mymodule`.

#### processResponse(data)

`data` may be `null` or whatever the processor specified in the `service` returned.

The `service` object defines a set of functions to allow you send information to the netdata server about:

1. Charts and dimension definitions
2. Updated values, from the collected values

---

*FIXME: document an operational node.d.plugin data collector - the best example is the [snmp collector](https://github.com/firehol/netdata/blob/master/node.d/snmp.node.js)*