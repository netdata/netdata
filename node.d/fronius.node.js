'use strict';

// This program will connect to one or more Fronius Symo Inverters.
// to get the Solar Power Generated (current, today).

// example configuration in /etc/netdata/node.d/fronius.conf
/*
 {
 "enable_autodetect": false,
 "update_every": 5,
 "servers": [
 {
 "name": "plant1",
 "hostname": "10.0.1.1",
 "update_every": 10,
 "api_path": "/solar_api/v1/GetPowerFlowRealtimeData.fcgi"
 },
 {
 "name": "plant2",
 "hostname": "10.0.2.1",
 "update_every": 15,
 "api_path": "/solar_api/v1/GetPowerFlowRealtimeData.fcgi"
 }
 ]
 }
 */


var url = require('url');
var http = require('http');
var netdata = require('netdata');

netdata.debug('loaded ' + __filename + ' plugin');

const power_grid_id = 'p_grid';
const power_pv_id = 'p_pv';
const power_accu_id = 'p_akku'; // not my typo! Using the ID from the API
const consumption_load_id = 'p_load';
const autonomy_id = 'rel_autonomy';
const consumption_self_id = 'rel_selfconsumption';

var fronius = {
    name: "Fronius",
    enable_autodetect: false,
    update_every: 5,
    base_priority: 60000,
    charts: {},

    createBasicDimension: function (id, name) {
        return {
            id: id,                                     // the unique id of the dimension
            name: name,                                 // the name of the dimension
            algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
            multiplier: 1,                              // the multiplier
            divisor: 1,                                 // the divisor
            hidden: false                               // is hidden (boolean)
        }
    },

    // Gets the site power chart. Will be created if not existing.
    getSitePowerChart: function (service, id) {

        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};
        dim[power_grid_id] = this.createBasicDimension(power_grid_id, "Grid");
        dim[power_pv_id] = this.createBasicDimension(power_pv_id, "Photovoltaics");
        dim[power_accu_id] = this.createBasicDimension(power_accu_id, "Accumulator");

        chart = {
            id: id,                                         // the unique id of the chart
            name: '',                                       // the unique name of the chart
            title: service.name + ' Current Site Power',    // the title of the chart
            units: 'W',                                   // the units of the chart dimensions
            family: 'Power',                                  // the family of the chart
            context: 'fronius.power',                // the context of the chart
            type: netdata.chartTypes.area,                  // the type of the chart
            priority: fronius.base_priority + 1,             // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(id, chart);
        fronius.charts[id] = chart;

        return chart;
    },

    // Gets the site consumption chart. Will be created if not existing.
    getSiteConsumptionChart: function (service, id) {

        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[consumption_load_id] = this.createBasicDimension(consumption_load_id, "Load");

        chart = {
            id: id,                                         // the unique id of the chart
            name: '',                                       // the unique name of the chart
            title: service.name + ' Current Load',          // the title of the chart
            units: 'W',                                     // the units of the chart dimensions
            family: 'Consumption',                                  // the family of the chart
            context: 'fronius.consumption',                 // the context of the chart
            type: netdata.chartTypes.area,                  // the type of the chart
            priority: fronius.base_priority + 2,            // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(id, chart);
        fronius.charts[id] = chart;

        return chart;
    },


    // Gets the site consumption chart. Will be created if not existing.
    getSiteAutonomyChart: function (service, id) {
        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[autonomy_id] = this.createBasicDimension(autonomy_id, "Autonomy");
        dim[consumption_self_id] = this.createBasicDimension(consumption_self_id, "Self Consumption");

        chart = {
            id: id,                                         // the unique id of the chart
            name: '',                                       // the unique name of the chart
            title: service.name + ' Current Autonomy',      // the title of the chart
            units: '%',                                     // the units of the chart dimensions
            family: 'Autonomy',                                  // the family of the chart
            context: 'fronius.autonomy',                 // the context of the chart
            type: netdata.chartTypes.area,                  // the type of the chart
            priority: fronius.base_priority + 3,            // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(id, chart);
        fronius.charts[id] = chart;

        return chart;
    },

    // Gets the inverter power chart. Will be created if not existing.
    // Needs the array of inverters in order to create a chart with all inverters as dimensions
    getInverterPowerChart: function (service, chartId, inverters) {

        var chart = fronius.charts[chartId];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};

        var inverter_count = Object.keys(inverters).length;
        var inverter = inverters[inverter_count.toString()];
        var i = 1;
        for (i; i <= inverter_count; i++) {
            if (fronius.isUndefined(inverter)) {
                netdata.error("Expected an Inverter with a numerical name! " +
                    "Have a look at your JSON output to verify.");
                continue;
            }
            dim[i.toString()] = this.createBasicDimension("inverter_" + i, "Inverter " + i);
        }

        chart = {
            id: chartId,                                         // the unique id of the chart
            name: '',                                       // the unique name of the chart
            title: service.name + ' Current Inverter Output',    // the title of the chart
            units: 'W',                                   // the units of the chart dimensions
            family: 'Inverters',                                  // the family of the chart
            context: 'fronius.inverter',                // the context of the chart
            type: netdata.chartTypes.stacked,                  // the type of the chart
            priority: fronius.base_priority + 4,             // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        fronius.charts[chartId] = chart;

        return chart;
    },

    // Gets the inverter energy production chart for today. Will be created if not existing.
    // Needs the array of inverters in order to create a chart with all inverters as dimensions
    getInverterEnergyTodayChart: function (service, chartId, inverters) {

        var chart = fronius.charts[chartId];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};

        var inverter_count = Object.keys(inverters).length;
        var inverter = inverters[inverter_count.toString()];
        var i = 1;
        for (i; i <= inverter_count; i++) {
            if (fronius.isUndefined(inverter)) {
                netdata.error("Expected an Inverter with a numerical name! " +
                    "Have a look at your JSON output to verify.");
                continue;
            }
            dim[i.toString()] = {
                id: 'inverter_' + i,           // the unique id of the dimension
                name: 'Inverter ' + i,  // the name of the dimension
                algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
                multiplier: 1,                              // the multiplier
                divisor: 1000,                                 // the divisor
                hidden: false                               // is hidden (boolean)
            };
        }

        chart = {
            id: chartId,                                         // the unique id of the chart
            name: '',                                       // the unique name of the chart
            title: service.name + ' Inverter Energy production for today',    // the title of the chart
            units: 'kWh',                                   // the units of the chart dimensions
            family: 'Inverters',                                  // the family of the chart
            context: 'fronius.inverter',                // the context of the chart
            type: netdata.chartTypes.stacked,                  // the type of the chart
            priority: fronius.base_priority + 5,             // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        fronius.charts[chartId] = chart;

        return chart;
    },


    processResponse: function (service, content) {
        if (content === null) return;
        var json = JSON.parse(content);
        // validating response
        if (fronius.isUndefined(json.Body)) return;
        if (fronius.isUndefined(json.Body.Data)) return;
        if (fronius.isUndefined(json.Body.Data.Site)) return;
        if (fronius.isUndefined(json.Body.Data.Inverters)) return;

        // add the service
        if (service.added !== true) service.commit();

        var site = json.Body.Data.Site;

        // Site Current Power Chart
        service.begin(fronius.getSitePowerChart(service, 'fronius_' + service.name + '.power'));
        service.set(power_grid_id, Math.round(site.P_Grid));
        service.set(power_pv_id, Math.round(site.P_PV));
        service.set(power_accu_id, Math.round(site.P_Akku));
        service.end();

        // Site Consumption Chart
        var consumption = site.P_Load;
        if (consumption === null) consumption = 0;
        consumption *= -1;

        service.begin(fronius.getSiteConsumptionChart(service, 'fronius_' + service.name + '.consumption'));
        service.set(consumption_load_id, Math.round(consumption));
        service.end();

        // Site Autonomy Chart
        service.begin(fronius.getSiteAutonomyChart(service, 'fronius_' + service.name + '.autonomy'));
        service.set(autonomy_id, Math.round(site.rel_Autonomy));
        service.set(consumption_self_id, Math.round(site.rel_SelfConsumption));
        service.end();

        // Inverters
        var inverters = json.Body.Data.Inverters;
        var inverter_count = Object.keys(inverters).length;
        if (inverter_count <= 0) return;
        var i = 1;
        for (i; i <= inverter_count; i++) {
            var inverter = inverters[i];
            if (fronius.isUndefined(inverter)) continue;
            netdata.debug("Setting values");
            service.begin(fronius.getInverterPowerChart(service, 'fronius_' + service.name + '.inverters.output', inverters));
            service.set(i.toString(), Math.round(inverter.P));
            service.end();
            service.begin(fronius.getInverterEnergyTodayChart(service, 'fronius_' + service.name + '.inverters.today', inverters));
            service.set(i.toString(), Math.round(inverter.E_Day));
            service.end();
        }
    },

    // module.serviceExecute()
    // this function is called only from this module
    // its purpose is to prepare the request and call
    // netdata.serviceExecute()
    serviceExecute: function (name, uri, update_every) {
        netdata.debug(this.name + ': ' + name + ': url: ' + uri + ', update_every: ' + update_every);

        var service = netdata.service({
            name: name,
            request: netdata.requestFromURL('http://' + uri),
            update_every: update_every,
            module: this
        });
        service.request.method = 'GET';
        service.execute(this.processResponse);
    },


    configure: function (config) {
        if (fronius.isUndefined(config.servers)) return 0;
        var added = 0;
        var len = config.servers.length;
        while (len--) {
            var server = config.servers[len];
            if (fronius.isUndefined(server.update_every)) server.update_every = this.update_every;

            var url = server.hostname + server.api_path;
            this.serviceExecute(server.name, url, server.update_every);
            added++;
        }
        return added;
    },

    // module.update()
    // this is called repeatedly to collect data, by calling
    // netdata.serviceExecute()
    update: function (service, callback) {
        service.execute(function (serv, data) {
            service.module.processResponse(serv, data);
            callback();
        });
    },

    isUndefined: function (value) {
        return typeof value === 'undefined';
    },

    isDefined: function (value) {
        return typeof value !== 'undefined';
    }


};

module.exports = fronius;
