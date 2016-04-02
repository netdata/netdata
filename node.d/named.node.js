'use strict';

// collect statistics from bind (named) v9.10+
//
// bind statistics documentation at:
// http://jpmens.net/2013/03/18/json-in-bind-9-s-statistics-server/
// https://ftp.isc.org/isc/bind/9.10.3/doc/arm/Bv9ARM.ch06.html#statistics

// example configuration in /etc/netdata/named.conf
// the module supports auto-detection if bind is running in localhost

/*
{
	"enable_autodetect": true,
	"update_every": 5,
	"servers": [
		{
			"name": "bind1",
			"url": "http://127.0.0.1:8888/json/v1/server",
			"update_every": 1
		},
		{
			"name": "bind2",
			"url": "http://10.0.0.1:8888/xml/v3/server",
			"update_every": 2
		}
	]
}
*/

// the following is the bind named.conf configuration required

/*
statistics-channels {
        inet 127.0.0.1 port 8888 allow { 127.0.0.1; };
};
*/

var url = require('url');
var http = require('http');
var XML = require('pixl-xml');
var netdata = require('netdata');

if(netdata.options.DEBUG === true) netdata.debug('loaded ' + __filename + ' plugin');

var named = {
	name: __filename,
	enable_autodetect: true,
	update_every: 1,
	base_priority: 60000,
	charts: {},

	chartFromMembersCreate: function(service, obj, id, title_suffix, units, family, context, type, priority, algorithm, multiplier, divisor) {
		var chart = {
			id: id,											// the unique id of the chart
			name: '',										// the unique name of the chart
			title: service.name + ' ' + title_suffix,		// the title of the chart
			units: units,									// the units of the chart dimensions
			family: family,									// the family of the chart
			context: context,								// the context of the chart
			type: type,										// the type of the chart
			priority: priority,								// the priority relative to others in the same family
			update_every: service.update_every, 			// the expected update frequency of the chart
			dimensions: {}
		}

		var found = 0;
		for(var x in obj) {
			if(typeof(obj[x]) !== 'undefined' && obj[x] !== 0) {
				found++;
				chart.dimensions[x] = {
					id: x,					// the unique id of the dimension
					name: x,				// the name of the dimension
					algorithm: algorithm,	// the id of the netdata algorithm
					multiplier: multiplier,	// the multiplier
					divisor: divisor,		// the divisor
					hidden: false			// is hidden (boolean)
				}
			}
		}

		if(found === false)
			return null;

		chart = service.chart(id, chart);
		this.charts[id] = chart;
		return chart;
	},

	chartFromMembers: function(service, obj, id_suffix, title_suffix, units, family, context, type, priority, algorithm, multiplier, divisor) {
		var id = 'named_' + service.name + '.' + id_suffix;
		var chart = this.charts[id];

		if(typeof chart === 'undefined') {
			chart = this.chartFromMembersCreate(service, obj, id, title_suffix, units, family, context, type, priority, algorithm, multiplier, divisor);
			if(chart === null) return false;
		}
		else {
			// check if we need to re-generate the chart
			for(var x in obj) {
				if(typeof(chart.dimensions[x]) === 'undefined') {
					chart = this.chartFromMembersCreate(service, obj, id, title_suffix, units, family, context, type, priority, algorithm, multiplier, divisor);
					if(chart === null) return false;
					break;
				}
			}
		}

		var found = 0;
		service.begin(chart);
		for(var x in obj) {
			if(typeof(chart.dimensions[x]) !== 'undefined') {
				found++;
				service.set(x, obj[x]);
			}
		}
		service.end();

		if(found > 0) return true;
		return false;
	},

	// an index to map values to different charts
	lookups: {
		nsstats: {},
		resolver_stats: {},
		numfetch: {}
	},

	// transform the XML response of bind
	// to the JSON response of bind
	xml2js: function(service, data_xml) {
		var d = XML.parse(data_xml);
		if(d === null) return null;

		var data = {};
		var len = d.server.counters.length;
		while(len--) {
			var a = d.server.counters[len];
			if(typeof a.counter === 'undefined') continue;
			if(a.type === 'opcode') a.type = 'opcodes';
			else if(a.type === 'qtype') a.type = 'qtypes';
			else if(a.type === 'nsstat') a.type = 'nsstats';
			var aa = data[a.type] = {};
			var alen = 0
			var alen2 = a.counter.length;
			while(alen < alen2) {
				aa[a.counter[alen].name] = parseInt(a.counter[alen]._Data);
				alen++;
			}
		}

		data.views = {};
		var vlen = d.views.view.length;
		while(vlen--) {
			var vname = d.views.view[vlen].name;
			data.views[vname] = { resolver: {} };
			var len = d.views.view[vlen].counters.length;
			while(len--) {
				var a = d.views.view[vlen].counters[len];
				if(typeof a.counter === 'undefined') continue;
				if(a.type === 'resstats') a.type = 'stats';
				else if(a.type === 'resqtype') a.type = 'qtypes';
				else if(a.type === 'adbstat') a.type = 'adb';
				var aa = data.views[vname].resolver[a.type] = {};
				var alen = 0;
				var alen2 = a.counter.length;
				while(alen < alen2) {
					aa[a.counter[alen].name] = parseInt(a.counter[alen]._Data);
					alen++;
				}
			}
		}

		return data;
	},

	processResponse: function(service, data) {
		if(data !== null) {
			var r;

			// parse XML or JSON
			// pepending on the URL given
			if(service.request.path.match(/^\/xml/) !== null)
				r = named.xml2js(service, data);
			else
				r = JSON.parse(data);

			if(typeof r === 'undefined' || r === null) {
				netdata.serviceError(service, "Cannot parse these data: " + data);
				return;
			}

			if(service.added !== true)
				service.commit();

			if(typeof r.nsstats !== 'undefined') {
				// we split the nsstats object to several others
				var global_requests = {}, global_requests_enable = false;
				var global_failures = {}, global_failures_enable = false;
				var global_failures_detail = {}, global_failures_detail_enable = false;
				var global_updates = {}, global_updates_enable = false;
				var protocol_queries = {}, protocol_queries_enable = false;
				var global_queries = {}, global_queries_enable = false;
				var global_queries_success = {}, global_queries_success_enable = false;
				var default_enable = false;
				var RecursClients = 0;

				// RecursClients is an absolute value
				if(typeof r.nsstats['RecursClients'] !== 'undefined') {
					RecursClients = r.nsstats['RecursClients'];
					delete r.nsstats['RecursClients'];
				}

				for( var x in r.nsstats ) {
					// we maintain an index of the values found
					// mapping them to objects splitted

					var look = named.lookups.nsstats[x];
					if(typeof look === 'undefined') {
						// a new value, not found in the index
						// index it:
						if(x === 'Requestv4') {
							named.lookups.nsstats[x] = {
								name: 'IPv4',
								type: 'global_requests'
							};
						}
						else if(x === 'Requestv6') {
							named.lookups.nsstats[x] = {
								name: 'IPv6',
								type: 'global_requests'
							};
						}
						else if(x === 'QryFailure') {
							named.lookups.nsstats[x] = {
								name: 'failures',
								type: 'global_failures'
							};
						}
						else if(x === 'QryUDP') {
							named.lookups.nsstats[x] = {
								name: 'UDP',
								type: 'protocol_queries'
							};
						}
						else if(x === 'QryTCP') {
							named.lookups.nsstats[x] = {
								name: 'TCP',
								type: 'protocol_queries'
							};
						}
						else if(x === 'QrySuccess') {
							named.lookups.nsstats[x] = {
								name: 'queries',
								type: 'global_queries_success'
							};
						}
						else if(x.match(/QryRej$/) !== null) {
							named.lookups.nsstats[x] = {
								name: x,
								type: 'global_failures_detail'
							};
						}
						else if(x.match(/^Qry/) !== null) {
							named.lookups.nsstats[x] = {
								name: x,
								type: 'global_queries'
							};
						}
						else if(x.match(/^Update/) !== null) {
							named.lookups.nsstats[x] = {
								name: x,
								type: 'global_updates'
							};
						}
						else {
							// values not mapped, will remain
							// in the default map
							named.lookups.nsstats[x] = {
								name: x,
								type: 'default'
							};
						}

						look = named.lookups.nsstats[x];
						// netdata.error('lookup nsstats value: ' + x + ' >>> ' + named.lookups.nsstats[x].type);
					}

					switch(look.type) {
						case 'global_requests': global_requests[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_requests_enable = true; break;
						case 'global_queries': global_queries[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_queries_enable = true; break;
						case 'global_queries_success': global_queries_success[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_queries_success_enable = true; break;
						case 'global_updates': global_updates[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_updates_enable = true; break;
						case 'protocol_queries': protocol_queries[look.name] = r.nsstats[x]; delete r.nsstats[x]; protocol_queries_enable = true; break;
						case 'global_failures': global_failures[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_failures_enable = true; break;
						case 'global_failures_detail': global_failures_detail[look.name] = r.nsstats[x]; delete r.nsstats[x]; global_failures_detail_enable = true; break;
						default: default_enable = true; break;
					}
				}

				if(global_requests_enable == true)
					service.module.chartFromMembers(service, global_requests, 'received_requests', 'Bind, Global Received Requests by IP version', 'requests/s', 'requests', 'named.requests', netdata.chartTypes.stacked, named.base_priority + 1, netdata.chartAlgorithms.incremental, 1, 1);

				if(global_queries_success_enable == true)
					service.module.chartFromMembers(service, global_queries_success, 'global_queries_success', 'Bind, Global Successful Queries', 'queries/s', 'queries', 'named.queries.succcess', netdata.chartTypes.line, named.base_priority + 2, netdata.chartAlgorithms.incremental, 1, 1);

				if(protocol_queries_enable == true)
					service.module.chartFromMembers(service, protocol_queries, 'protocols_queries', 'Bind, Global Queries by IP Protocol', 'queries/s', 'queries', 'named.protocol.queries', netdata.chartTypes.stacked, named.base_priority + 3, netdata.chartAlgorithms.incremental, 1, 1);

				if(global_queries_enable == true)
					service.module.chartFromMembers(service, global_queries, 'global_queries', 'Bind, Global Queries Analysis', 'queries/s', 'queries', 'named.global.queries', netdata.chartTypes.stacked, named.base_priority + 4, netdata.chartAlgorithms.incremental, 1, 1);

				if(global_updates_enable == true)
					service.module.chartFromMembers(service, global_updates, 'received_updates', 'Bind, Global Received Updates', 'updates/s', 'updates', 'named.global.updates', netdata.chartTypes.stacked, named.base_priority + 5, netdata.chartAlgorithms.incremental, 1, 1);

				if(global_failures_enable == true)
					service.module.chartFromMembers(service, global_failures, 'query_failures', 'Bind, Global Query Failures', 'failures/s', 'failures', 'named.global.failures', netdata.chartTypes.line, named.base_priority + 6, netdata.chartAlgorithms.incremental, 1, 1);

				if(global_failures_detail_enable == true)
					service.module.chartFromMembers(service, global_failures_detail, 'query_failures_detail', 'Bind, Global Query Failures Analysis', 'failures/s', 'failures', 'named.global.failures.detail', netdata.chartTypes.stacked, named.base_priority + 7, netdata.chartAlgorithms.incremental, 1, 1);

				if(default_enable === true)
					service.module.chartFromMembers(service, r.nsstats, 'nsstats', 'Bind, Other Global Server Statistics', 'operations/s', 'other', 'named.nsstats', netdata.chartTypes.line, named.base_priority + 8, netdata.chartAlgorithms.incremental, 1, 1);

				// RecursClients chart
				{
					var id = 'named_' + service.name + '.recursive_clients';
					var chart = named.charts[id];

					if(typeof chart === 'undefined') {
						chart = {
							id: id,											// the unique id of the chart
							name: '',										// the unique name of the chart
							title: service.name + ' Bind, Current Recursive Clients',		// the title of the chart
							units: 'clients',								// the units of the chart dimensions
							family: 'clients',								// the family of the chart
							context: 'named.recursive.clients',				// the context of the chart
							type: netdata.chartTypes.line,					// the type of the chart
							priority: named.base_priority + 1,				// the priority relative to others in the same family
							update_every: service.update_every,				// the expected update frequency of the chart
							dimensions: {
								'clients': {
									id: 'clients',								// the unique id of the dimension
									name: '',									// the name of the dimension
									algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
									multiplier: 1,								// the multiplier
									divisor: 1,									// the divisor
									hidden: false								// is hidden (boolean)
								}
							}
						};

						chart = service.chart(id, chart);
						named.charts[id] = chart;
					}

					service.begin(chart);
					service.set('clients', RecursClients);
					service.end();
				}
			}

			if(typeof r.opcodes !== 'undefined')
				service.module.chartFromMembers(service, r.opcodes, 'in_opcodes', 'Bind, Global Incoming Requests by OpCode', 'requests/s', 'requests', 'named.in.opcodes', netdata.chartTypes.stacked, named.base_priority + 9, netdata.chartAlgorithms.incremental, 1, 1);

			if(typeof r.qtypes !== 'undefined')
				service.module.chartFromMembers(service, r.qtypes, 'in_qtypes', 'Bind, Global Incoming Requests by Query Type', 'requests/s', 'requests', 'named.in.qtypes', netdata.chartTypes.stacked, named.base_priority + 10, netdata.chartAlgorithms.incremental, 1, 1);

			if(typeof r.sockstats !== 'undefined')
				service.module.chartFromMembers(service, r.sockstats, 'in_sockstats', 'Bind, Global Socket Statistics', 'operations/s', 'sockets', 'named.in.sockstats', netdata.chartTypes.line, named.base_priority + 11, netdata.chartAlgorithms.incremental, 1, 1);

			if(typeof r.views !== 'undefined') {
				for( var x in r.views ) {
					var resolver = r.views[x].resolver;

					if(typeof resolver !== 'undefined') {
						if(typeof resolver.stats !== 'undefined') {
							var NumFetch = 0;
							var key = service.name + '.' + x;
							var default_enable = false;
							var rtt = {}, rtt_enable = false;

							// NumFetch is an absolute value
							if(typeof resolver.stats['NumFetch'] !== 'undefined') {
								named.lookups.numfetch[key] = true;
								NumFetch = resolver.stats['NumFetch'];
								delete resolver.stats['NumFetch'];
							}
							if(typeof resolver.stats['BucketSize'] !== 'undefined') {
								delete resolver.stats['BucketSize'];
							}

							// split the QryRTT* from the main chart
							for( var y in resolver.stats ) {
								// we maintain an index of the values found
								// mapping them to objects splitted

								var look = named.lookups.resolver_stats[y];
								if(typeof look === 'undefined') {
									if(y.match(/^QryRTT/) !== null) {
										named.lookups.resolver_stats[y] = {
											name: y,
											type: 'rtt'
										};
									}
									else {
										named.lookups.resolver_stats[y] = {
											name: y,
											type: 'default'
										};
									}

									look = named.lookups.resolver_stats[y];
									// netdata.error('lookup resolver stats value: ' + y + ' >>> ' + look.type);
								}

								switch(look.type) {
									case 'rtt': rtt[look.name] = resolver.stats[y]; delete resolver.stats[y]; rtt_enable = true; break;
									default: default_enable = true; break;
								}
							}

							if(rtt_enable)
								service.module.chartFromMembers(service, rtt, 'view_resolver_rtt_' + x, 'Bind, ' + x + ' View, Resolver Round Trip Timings', 'queries/s', 'view_' + x, 'named.resolver.rtt', netdata.chartTypes.stacked, named.base_priority + 12, netdata.chartAlgorithms.incremental, 1, 1);

							if(default_enable)
								service.module.chartFromMembers(service, resolver.stats, 'view_resolver_stats_' + x, 'Bind, ' + x + ' View, Resolver Statistics', 'operations/s', 'view_' + x, 'named.resolver.stats', netdata.chartTypes.line, named.base_priority + 13, netdata.chartAlgorithms.incremental, 1, 1);

							// NumFetch chart
							if(typeof named.lookups.numfetch[key] !== 'undefined') {
								var id = 'named_' + service.name + '.view_resolver_numfetch_' + x;
								var chart = named.charts[id];

								if(typeof chart === 'undefined') {
									chart = {
										id: id,											// the unique id of the chart
										name: '',										// the unique name of the chart
										title: service.name + ' Bind, ' + x + ' View, Resolver Active Queries',		// the title of the chart
										units: 'queries',								// the units of the chart dimensions
										family: 'view_' + x,							// the family of the chart
										context: 'named.resolver.active.queries',		// the context of the chart
										type: netdata.chartTypes.line,					// the type of the chart
										priority: named.base_priority + 1001,			// the priority relative to others in the same family
										update_every: service.update_every,				// the expected update frequency of the chart
										dimensions: {
											'queries': {
												id: 'queries',								// the unique id of the dimension
												name: '',									// the name of the dimension
												algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
												multiplier: 1,								// the multiplier
												divisor: 1,									// the divisor
												hidden: false								// is hidden (boolean)
											}
										}
									};

									chart = service.chart(id, chart);
									named.charts[id] = chart;
								}

								service.begin(chart);
								service.set('queries', NumFetch);
								service.end();
							}
						}
					}

					if(typeof resolver.qtypes !== 'undefined')
						service.module.chartFromMembers(service, resolver.qtypes, 'view_resolver_qtypes_' + x, 'Bind, ' + x + ' View, Requests by Query Type', 'requests/s', 'view_' + x, 'named.resolver.qtypes', netdata.chartTypes.stacked, named.base_priority + 14, netdata.chartAlgorithms.incremental, 1, 1);

					//if(typeof resolver.cache !== 'undefined')
					//	service.module.chartFromMembers(service, resolver.cache, 'view_resolver_cache_' + x, 'Bind, ' + x + ' View, Cache Entries', 'entries', 'view_' + x, 'named.resolver.cache', netdata.chartTypes.stacked, named.base_priority + 15, netdata.chartAlgorithms.absolute, 1, 1);

					if(typeof resolver.cachestats['CacheHits'] !== 'undefined' && resolver.cachestats['CacheHits'] > 0) {
						var id = 'named_' + service.name + '.view_resolver_cachehits_' + x;
						var chart = named.charts[id];

						if(typeof chart === 'undefined') {
							chart = {
								id: id,											// the unique id of the chart
								name: '',										// the unique name of the chart
								title: service.name + ' Bind, ' + x + ' View, Resolver Cache Hits',		// the title of the chart
								units: 'operations/s',							// the units of the chart dimensions
								family: 'view_' + x,							// the family of the chart
								context: 'named.resolver.cache.hits',			// the context of the chart
								type: netdata.chartTypes.area,					// the type of the chart
								priority: named.base_priority + 1100,			// the priority relative to others in the same family
								update_every: service.update_every,				// the expected update frequency of the chart
								dimensions: {
									'CacheHits': {
										id: 'CacheHits',							// the unique id of the dimension
										name: 'hits',								// the name of the dimension
										algorithm: netdata.chartAlgorithms.incremental,// the id of the netdata algorithm
										multiplier: 1,								// the multiplier
										divisor: 1,									// the divisor
										hidden: false								// is hidden (boolean)
									},
									'CacheMisses': {
										id: 'CacheMisses',							// the unique id of the dimension
										name: 'misses',								// the name of the dimension
										algorithm: netdata.chartAlgorithms.incremental,// the id of the netdata algorithm
										multiplier: -1,								// the multiplier
										divisor: 1,									// the divisor
										hidden: false								// is hidden (boolean)
									}
								}
							};

							chart = service.chart(id, chart);
							named.charts[id] = chart;
						}

						service.begin(chart);
						service.set('CacheHits', resolver.cachestats['CacheHits']);
						service.set('CacheMisses', resolver.cachestats['CacheMisses']);
						service.end();
					}

					// this is wrong, it contains many types of info:
					// 1. CacheHits, CacheMisses - incremental (added above)
					// 2. QueryHits, QueryMisses - incremental
					// 3. DeleteLRU, DeleteTTL - incremental
					// 4. CacheNodes, CacheBuckets - absolute
					// 5. TreeMemTotal, TreeMemInUse - absolute
					// 6. HeapMemMax, HeapMemTotal, HeapMemInUse - absolute
					//if(typeof resolver.cachestats !== 'undefined')
					//	service.module.chartFromMembers(service, resolver.cachestats, 'view_resolver_cachestats_' + x, 'Bind, ' + x + ' View, Cache Statistics', 'requests/s', 'view_' + x, 'named.resolver.cache.stats', netdata.chartTypes.line, named.base_priority + 1001, netdata.chartAlgorithms.incremental, 1, 1);

					//if(typeof resolver.adb !== 'undefined')
					//	service.module.chartFromMembers(service, resolver.adb, 'view_resolver_adb_' + x, 'Bind, ' + x + ' View, ADB Statistics', 'entries', 'view_' + x, 'named.resolver.adb', netdata.chartTypes.line, named.base_priority + 1002, netdata.chartAlgorithms.absolute, 1, 1);
				}
			}
		}
	},

	// module.serviceExecute()
	// this function is called only from this module
	// its purpose is to prepare the request and call
	// netdata.serviceExecute()
	serviceExecute: function(name, a_url, update_every) {
		if(netdata.options.DEBUG === true) netdata.debug(this.name + ': ' + name + ': url: ' + a_url + ', update_every: ' + update_every);
		var service = netdata.service({
			name: name,
			request: netdata.requestFromURL(a_url),
			update_every: update_every,
			module: this
		});

		service.execute(this.processResponse);
	},

	configure: function(config) {
		var added = 0;

		if(this.enable_autodetect === true) {
			this.serviceExecute('local', 'http://localhost:8888/json/v1/server', this.update_every);
			added++;
		}
		
		if(typeof(config.servers) !== 'undefined') {
			var len = config.servers.length;
			while(len--) {
				if(typeof config.servers[len].update_every === 'undefined')
					config.servers[len].update_every = this.update_every;

				this.serviceExecute(config.servers[len].name, config.servers[len].url, config.servers[len].update_every);
				added++;
			}
		}

		return added;
	},

	// module.update()
	// this is called repeatidly to collect data, by calling
	// netdata.serviceExecute()
	update: function(service, callback) {
		service.execute(function(serv, data) {
			service.module.processResponse(serv, data);
			callback();
		});
	},
};

module.exports = named;
