'use strict';
// SPDX-License-Identifier: GPL-3.0+

// This program will connect to one or more SMA Sunny Webboxes
// to get the Solar Power Generated (current, today, total).

// example configuration in /etc/netdata/node.d/sma_webbox.conf
/*
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "plant1",
            "hostname": "10.0.1.1",
            "update_every": 10
        },
        {
            "name": "plant2",
            "hostname": "10.0.2.1",
            "update_every": 15
        }
    ]
}
*/

var url = require('url');
var http = require('http');
var netdata = require('netdata');

if(netdata.options.DEBUG === true) netdata.debug('loaded ' + __filename + ' plugin');

var webbox = {
    name: __filename,
    enable_autodetect: true,
    update_every: 1,
    base_priority: 60000,
    charts: {},

    processResponse: function(service, data) {
        if(data !== null) {
            var r = JSON.parse(data);

            var d = {
                'GriPwr': {
                    unit: null,
                    value: null
                },
                'GriEgyTdy': {
                    unit: null,
                    value: null
                },
                'GriEgyTot': {
                    unit: null,
                    value: null
                }
            };

            // parse the webbox response
            // and put it in our d object
            var found = 0;
            var len = r.result.overview.length;
            while(len--) {
                var e = r.result.overview[len];
                if(typeof(d[e.meta]) !== 'undefined') {
                    found++;
                    d[e.meta].value = e.value;
                    d[e.meta].unit = e.unit;
                }
            }

            // add the service
            if(found > 0 && service.added !== true)
                service.commit();

            // Grid Current Power Chart
            if(d['GriPwr'].value !== null) {
                var id = 'smawebbox_' + service.name + '.current';
                var chart = webbox.charts[id];

                if(typeof chart === 'undefined') {
                    chart = {
                        id: id,                                         // the unique id of the chart
                        name: '',                                       // the unique name of the chart
                        title: service.name + ' Current Grid Power',    // the title of the chart
                        units: d['GriPwr'].unit,                        // the units of the chart dimensions
                        family: 'now',                                  // the family of the chart
                        context: 'smawebbox.grid_power',                // the context of the chart
                        type: netdata.chartTypes.area,                  // the type of the chart
                        priority: webbox.base_priority + 1,             // the priority relative to others in the same family
                        update_every: service.update_every,             // the expected update frequency of the chart
                        dimensions: {
                            'GriPwr': {
                                id: 'GriPwr',                               // the unique id of the dimension
                                name: 'power',                              // the name of the dimension
                                algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
                                multiplier: 1,                              // the multiplier
                                divisor: 1,                                 // the divisor
                                hidden: false                               // is hidden (boolean)
                            }
                        }
                    };

                    chart = service.chart(id, chart);
                    webbox.charts[id] = chart;
                }

                service.begin(chart);
                service.set('GriPwr', Math.round(d['GriPwr'].value));
                service.end();
            }

            if(d['GriEgyTdy'].value !== null) {
                var id = 'smawebbox_' + service.name + '.today';
                var chart = webbox.charts[id];

                if(typeof chart === 'undefined') {
                    chart = {
                        id: id,                                         // the unique id of the chart
                        name: '',                                       // the unique name of the chart
                        title: service.name + ' Today Grid Power',      // the title of the chart
                        units: d['GriEgyTdy'].unit,                     // the units of the chart dimensions
                        family: 'today',                                // the family of the chart
                        context: 'smawebbox.grid_power_today',          // the context of the chart
                        type: netdata.chartTypes.area,                  // the type of the chart
                        priority: webbox.base_priority + 2,             // the priority relative to others in the same family
                        update_every: service.update_every,             // the expected update frequency of the chart
                        dimensions: {
                            'GriEgyTdy': {
                                id: 'GriEgyTdy',                                // the unique id of the dimension
                                name: 'power',                              // the name of the dimension
                                algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
                                multiplier: 1,                              // the multiplier
                                divisor: 1000,                              // the divisor
                                hidden: false                               // is hidden (boolean)
                            }
                        }
                    };

                    chart = service.chart(id, chart);
                    webbox.charts[id] = chart;
                }

                service.begin(chart);
                service.set('GriEgyTdy', Math.round(d['GriEgyTdy'].value * 1000));
                service.end();
            }

            if(d['GriEgyTot'].value !== null) {
                var id = 'smawebbox_' + service.name + '.total';
                var chart = webbox.charts[id];

                if(typeof chart === 'undefined') {
                    chart = {
                        id: id,                                         // the unique id of the chart
                        name: '',                                       // the unique name of the chart
                        title: service.name + ' Total Grid Power',      // the title of the chart
                        units: d['GriEgyTot'].unit,                     // the units of the chart dimensions
                        family: 'total',                                // the family of the chart
                        context: 'smawebbox.grid_power_total',          // the context of the chart
                        type: netdata.chartTypes.area,                  // the type of the chart
                        priority: webbox.base_priority + 3,             // the priority relative to others in the same family
                        update_every: service.update_every,             // the expected update frequency of the chart
                        dimensions: {
                            'GriEgyTot': {
                                id: 'GriEgyTot',                                // the unique id of the dimension
                                name: 'power',                              // the name of the dimension
                                algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
                                multiplier: 1,                              // the multiplier
                                divisor: 1000,                              // the divisor
                                hidden: false                               // is hidden (boolean)
                            }
                        }
                    };

                    chart = service.chart(id, chart);
                    webbox.charts[id] = chart;
                }

                service.begin(chart);
                service.set('GriEgyTot', Math.round(d['GriEgyTot'].value * 1000));
                service.end();
            }
        }
    },

    // module.serviceExecute()
    // this function is called only from this module
    // its purpose is to prepare the request and call
    // netdata.serviceExecute()
    serviceExecute: function(name, hostname, update_every) {
        if(netdata.options.DEBUG === true) netdata.debug(this.name + ': ' + name + ': hostname: ' + hostname + ', update_every: ' + update_every);

        var service = netdata.service({
            name: name,
            request: netdata.requestFromURL('http://' + hostname + '/rpc'),
            update_every: update_every,
            module: this
        });
        service.postData = 'RPC={"proc":"GetPlantOverview","format":"JSON","version":"1.0","id":"1"}';
        service.request.method = 'POST';
        service.request.headers['Content-Length'] = service.postData.length;

        service.execute(this.processResponse);
    },

    configure: function(config) {
        var added = 0;

        if(typeof(config.servers) !== 'undefined') {
            var len = config.servers.length;
            while(len--) {
                if(typeof config.servers[len].update_every === 'undefined')
                    config.servers[len].update_every = this.update_every;

                if(config.servers[len].update_every < 5)
                    config.servers[len].update_every = 5;

                this.serviceExecute(config.servers[len].name, config.servers[len].hostname, config.servers[len].update_every);
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

module.exports = webbox;
