'use strict';

// This program will connect to one or more SNMP Agents

// example configuration in /etc/netdata/snmp.conf
/*
{
	"enable_autodetect": false,
	"update_every": 5,
	"max_request_size": 50,
	"servers": [
		{
			"hostname": "10.11.12.8",
			"community": "public",
			"update_every": 10,
			"max_request_size": 50,
			"options": { "timeout": 10000 },
			"charts": {
				"snmp_switch.bandwidth_port1": {
					"title": "Switch Bandwidth for port 1",
					"units": "kilobits/s",
					"type": "area",
					"priority": 1,
					"dimensions": {
						"in": {
							"oid": ".1.3.6.1.2.1.2.2.1.10.1",
							"algorithm": "incremental",
							"multiplier": 8,
							"divisor": 1024
						},
						"out": {
							"oid": ".1.3.6.1.2.1.2.2.1.16.1",
							"algorithm": "incremental",
							"multiplier": -8,
							"divisor": 1024
						}
					}
				},
				"snmp_switch.bandwidth_port2": {
					"title": "Switch Bandwidth for port 2",
					"units": "kilobits/s",
					"type": "area",
					"priority": 1,
					"dimensions": {
						"in": {
							"oid": ".1.3.6.1.2.1.2.2.1.10.2",
							"algorithm": "incremental",
							"multiplier": 8,
							"divisor": 1024
						},
						"out": {
							"oid": ".1.3.6.1.2.1.2.2.1.16.2",
							"algorithm": "incremental",
							"multiplier": -8,
							"divisor": 1024
						}
					}
				}
			}
		}
	]
}
*/

// You can also give ranges of charts like the following.
// This will append 1-24 to id, title, oid (on each dimension)
// so that 24 charts will be created.
/*
{
	"enable_autodetect": false,
	"update_every": 10,
	"max_request_size": 50,
	"servers": [
		{
			"hostname": "10.11.12.8",
			"community": "public",
			"update_every": 10,
			"max_request_size": 50,
			"options": { "timeout": 20000 },
			"charts": {
				"snmp_switch.bandwidth_port": {
					"title": "Switch Bandwidth for port ",
					"units": "kilobits/s",
					"type": "area",
					"priority": 1,
					"multiply_range": [ 1, 24 ],
					"dimensions": {
						"in": {
							"oid": ".1.3.6.1.2.1.2.2.1.10.",
							"algorithm": "incremental",
							"multiplier": 8,
							"divisor": 1024
						},
						"out": {
							"oid": ".1.3.6.1.2.1.2.2.1.16.",
							"algorithm": "incremental",
							"multiplier": -8,
							"divisor": 1024
						}
					}
				}
			}
		}
	]
}
*/

var net_snmp = require('net-snmp');
var extend = require('extend');
var netdata = require('netdata');

if(netdata.options.DEBUG === true) netdata.debug('loaded ' + __filename + ' plugin');

netdata.processors.snmp = {
	name: 'snmp',

	fixoid: function(oid) {
		if(typeof oid !== 'string')
			return oid;

		if(oid.charAt(0) === '.')
			return oid.substring(1, oid.length);

		return oid;
	},

	prepare: function(service) {
		if(typeof service.snmp_oids === 'undefined' || service.snmp_oids === null || service.snmp_oids.length === 0) {
			// this is the first time we see this service

			if(netdata.options.DEBUG === true)
				netdata.debug(service.module.name + ': ' + service.name + ': preparing ' + this.name + ' OIDs');

			// build an index of all OIDs
			service.snmp_oids_index = {};
			for(var c in service.request.charts) {
				// for each chart

				if(netdata.options.DEBUG === true)
					netdata.debug(service.module.name + ': ' + service.name + ': indexing ' + this.name + ' chart: ' + c);

				if(typeof service.request.charts[c].titleoid !== 'undefined') {
						service.snmp_oids_index[this.fixoid(service.request.charts[c].titleoid)] = {
							type: 'title',
							link: service.request.charts[c]
						};
					}

				for(var d in service.request.charts[c].dimensions) {
					// for each dimension in the chart

					var oid = this.fixoid(service.request.charts[c].dimensions[d].oid);
					var oidname = this.fixoid(service.request.charts[c].dimensions[d].oidname);
					
					if(netdata.options.DEBUG === true)
						netdata.debug(service.module.name + ': ' + service.name + ': indexing ' + this.name + ' chart: ' + c + ', dimension: ' + d + ', OID: ' + oid + ", OID name: " + oidname);

					// link it to the point we need to set the value to
					service.snmp_oids_index[oid] = {
						type: 'value',
						link: service.request.charts[c].dimensions[d]
					};

					if(typeof oidname !== 'undefined')
						service.snmp_oids_index[oidname] = {
							type: 'name',
							link: service.request.charts[c].dimensions[d]
						};

					// and set the value to null
					service.request.charts[c].dimensions[d].value = null;
				}
			}

			if(netdata.options.DEBUG === true)
				netdata.debug(service.module.name + ': ' + service.name + ': indexed ' + this.name + ' OIDs: ' + netdata.stringify(service.snmp_oids_index));

			// now create the array of OIDs needed by net-snmp
			service.snmp_oids = new Array();
			for(var o in service.snmp_oids_index)
				service.snmp_oids.push(o);

			if(netdata.options.DEBUG === true)
				netdata.debug(service.module.name + ': ' + service.name + ': final list of ' + this.name + ' OIDs: ' + netdata.stringify(service.snmp_oids));

			service.snmp_oids_cleaned = 0;
		}
		else if(service.snmp_oids_cleaned === 0) {
			service.snmp_oids_cleaned = 1;

			// the second time, keep only values
			service.snmp_oids = new Array();
			for(var o in service.snmp_oids_index)
				if(service.snmp_oids_index[o].type === 'value')
					service.snmp_oids.push(o);
		}
	},

	getdata: function(service, index, ok, failed, callback) {
		var that = this;

		if(index >= service.snmp_oids.length) {
			callback((ok > 0)?{ ok: ok, failed: failed }:null);
			return;
		}

		var slice;
		if(service.snmp_oids.length <= service.request.max_request_size) {
			slice = service.snmp_oids;
			index = service.snmp_oids.length;
		}
		else if(service.snmp_oids.length - index <= service.request.max_request_size) {
			slice = service.snmp_oids.slice(index, service.snmp_oids.length);
			index = service.snmp_oids.length;
		}
		else {
			slice = service.snmp_oids.slice(index, index + service.request.max_request_size);
			index += service.request.max_request_size;
		}

		if(netdata.options.DEBUG === true)
			netdata.debug(service.module.name + ': ' + service.name + ': making ' + slice.length + ' entries request, max is: ' + service.request.max_request_size);

		service.snmp_session.get(slice, function(error, varbinds) {
			if(error) {
				service.error('Received error = ' + netdata.stringify(error) + ' varbinds = ' + netdata.stringify(varbinds));

				// make all values null
				var len = slice.length;
				while(len--)
					service.snmp_oids_index[slice[len]].value = null;
			}
			else {
				if(netdata.options.DEBUG === true)
					netdata.debug(service.module.name + ': ' + service.name + ': got valid ' + service.module.name + ' response: ' + netdata.stringify(varbinds));

				for(var i = 0; i < varbinds.length; i++) {
					var value = null;

					if(net_snmp.isVarbindError(varbinds[i])) {
						if(netdata.options.DEBUG === true)
							netdata.debug(service.module.name + ': ' + service.name + ': failed ' + service.module.name + ' get for OIDs ' + varbinds[i].oid);

						service.error('OID ' + varbinds[i].oid + ' gave error: ' + snmp.varbindError(varbinds[i]));
						value = null;
						failed++;
					}
					else {
						if(netdata.options.DEBUG === true)
							netdata.debug(service.module.name + ': ' + service.name + ': found ' + service.module.name + ' value of OIDs ' + varbinds[i].oid + " = " + varbinds[i].value);

						value = varbinds[i].value;
						ok++;
					}

					if(value !== null) {
						switch(service.snmp_oids_index[varbinds[i].oid].type) {
							case 'title': service.snmp_oids_index[varbinds[i].oid].link.title += ' ' + value; break;
							case 'name' : service.snmp_oids_index[varbinds[i].oid].link.name = value; break;
							case 'value': service.snmp_oids_index[varbinds[i].oid].link.value = value; break;
						}
					}
				}

				if(netdata.options.DEBUG === true)
					netdata.debug(service.module.name + ': ' + service.name + ': finished ' + service.module.name + ' with ' + ok + ' successful and ' + failed + ' failed values');
			}
			that.getdata(service, index, ok, failed, callback);
		});
	},

	process: function(service, callback) {
		this.prepare(service);

		if(service.snmp_oids.length === 0) {
			// no OIDs found for this service

			if(netdata.options.DEBUG === true)
				service.error('no OIDs to process.');

			callback(null);
			return;
		}

		if(typeof service.snmp_session === 'undefined' || service.snmp_session === null) {
			// no SNMP session has been created for this service
			// the SNMP session is just the initialization of NET-SNMP

			if(netdata.options.DEBUG === true)
				netdata.debug(service.module.name + ': ' + service.name + ': opening ' + this.name + ' session on ' + service.request.hostname + ' community ' + service.request.community + ' options ' + netdata.stringify(service.request.options));

			// create the SNMP session
			service.snmp_session = net_snmp.createSession (service.request.hostname, service.request.community, service.request.options);

			if(netdata.options.DEBUG === true)
				netdata.debug(service.module.name + ': ' + service.name + ': got ' + this.name + ' session: ' + netdata.stringify(service.snmp_session));

			// if we later need traps, this is how to do it:
			//service.snmp_session.trap(net_snmp.TrapType.LinkDown, function(error) {
			//	if(error) console.error('trap error: ' + netdata.stringify(error));
			//});
		}

		// do it, get the SNMP values for the sessions we need
		this.getdata(service, 0, 0, 0, callback);
	}
};

var snmp = {
	name: __filename,
	enable_autodetect: true,
	update_every: 1,
	base_priority: 50000,

	charts: {},

	processResponse: function(service, data) {
		if(data !== null) {
			if(service.added !== true)
				service.commit();

			for(var c in service.request.charts) {
				var chart = snmp.charts[c];

				if(typeof chart === 'undefined') {
					chart = service.chart(c, service.request.charts[c]);
					snmp.charts[c] = chart;
				}

				service.begin(chart);
				
				for( var d in service.request.charts[c].dimensions )
					if(service.request.charts[c].dimensions[d].value !== null)
						service.set(d, service.request.charts[c].dimensions[d].value);

				service.end();
			}
		}
	},

	// module.serviceExecute()
	// this function is called only from this module
	// its purpose is to prepare the request and call
	// netdata.serviceExecute()
	serviceExecute: function(conf) {
		if(netdata.options.DEBUG === true)
			netdata.debug(this.name + ': snmp hostname: ' + conf.hostname + ', update_every: ' + conf.update_every);

		var service = netdata.service({
			name: conf.hostname,
			request: conf,
			update_every: conf.update_every,
			module: this,
			processor: netdata.processors.snmp
		});

		// multiply the charts, if required
		for(var c in service.request.charts) {
			if(netdata.options.DEBUG === true)
				netdata.debug(this.name + ': snmp hostname: ' + conf.hostname + ', examining chart: ' + c);

			if(typeof service.request.charts[c].update_every === 'undefined')
				service.request.charts[c].update_every = service.update_every;

			if(typeof service.request.charts[c].multiply_range !== 'undefined') {
				var from = service.request.charts[c].multiply_range[0];
				var to = service.request.charts[c].multiply_range[1];
				var prio = service.request.charts[c].priority || 1;

				if(prio < snmp.base_priority) prio += snmp.base_priority;

				while(from <= to) {
					var id = c + from.toString();
					var chart = extend(true, {}, service.request.charts[c]);
					chart.title += from.toString();
					
					if(typeof chart.titleoid !== 'undefined')
						chart.titleoid += from.toString();

					chart.priority = prio++;
					for(var d in chart.dimensions) {
						chart.dimensions[d].oid += from.toString();

						if(typeof chart.dimensions[d].oidname !== 'undefined')
							chart.dimensions[d].oidname += from.toString();
					}
					service.request.charts[id] = chart;
					from++;
				}

				delete service.request.charts[c];
			}
			else {
				if(service.request.charts[c].priority < snmp.base_priority)
					service.request.charts[c].priority += snmp.base_priority;
			}
		}

		service.execute(this.processResponse);
	},

	configure: function(config) {
		var added = 0;

		if(typeof config.max_request_size === 'undefined')
			config.max_request_size = 50;

		if(typeof(config.servers) !== 'undefined') {
			var len = config.servers.length;
			while(len--) {
				if(typeof config.servers[len].update_every === 'undefined')
					config.servers[len].update_every = this.update_every;

				if(typeof config.servers[len].max_request_size === 'undefined')
					config.servers[len].max_request_size = config.max_request_size;

				this.serviceExecute(config.servers[len]);
				added++;
			}
		}

		return added;
	},

	// module.update()
	// this is called repeatidly to collect data, by calling
	// service.execute()
	update: function(service, callback) {
		service.execute(function(serv, data) {
			service.module.processResponse(serv, data);
			callback();
		});
	},
};

module.exports = snmp;
