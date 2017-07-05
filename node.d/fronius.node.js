'use strict';

// This program will connect to one or more Fronius Symo Inverters.
// to get the Solar Power Generated (current, today).

// example configuration in netdata/conf.d/node.d/fronius.conf.md

var url = require('url');
var http = require('http');
var netdata = require('netdata');

netdata.debug('loaded ' + __filename + ' plugin');

var fronius = {
    name: "Fronius",
    enable_autodetect: false,
    update_every: 5,
    base_priority: 60000,
    charts: {},

    powerGridId: "p_grid",
    powerPvId: 'p_pv',
    powerAccuId: 'p_akku', // not my typo! Using the ID from the AP
    consumptionLoadId: 'p_load',
    autonomyId: 'rel_autonomy',
    consumptionSelfId: 'rel_selfconsumption',

    createBasicDimension: function (id, name) {
        return {
            id: id,                                     // the unique id of the dimension
            name: name,                                 // the name of the dimension
            algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
            multiplier: 1,                              // the multiplier
            divisor: 1,                                 // the divisor
            hidden: false                               // is hidden (boolean)
        };
    },

    // Gets the site power chart. Will be created if not existing.
    getSitePowerChart: function (service, id) {

        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};
        dim[fronius.powerGridId] = this.createBasicDimension(fronius.powerGridId, "Grid");
        dim[fronius.powerPvId] = this.createBasicDimension(fronius.powerPvId, "Photovoltaics");
        dim[fronius.powerAccuId] = this.createBasicDimension(fronius.powerAccuId, "Accumulator");

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
        dim[fronius.consumptionLoadId] = this.createBasicDimension(fronius.consumptionLoadId, "Load");

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
        dim[fronius.autonomyId] = this.createBasicDimension(fronius.autonomyId, "Autonomy");
        dim[fronius.consumptionSelfId] = this.createBasicDimension(fronius.consumptionSelfId, "Self Consumption");

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

        var inverterCount = Object.keys(inverters).length;
        var inverter = inverters[inverterCount.toString()];
        var i = 1;
        for (i; i <= inverterCount; i++) {
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

        var inverterCount = Object.keys(inverters).length;
        var inverter = inverters[inverterCount.toString()];
        var i = 1;
        for (i; i <= inverterCount; i++) {
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
        service.set(fronius.powerGridId, Math.round(site.P_Grid));
        service.set(fronius.powerPvId, Math.round(site.P_PV));
        service.set(fronius.powerAccuId, Math.round(site.P_Akku));
        service.end();

        // Site Consumption Chart
        var consumption = site.P_Load;
        if (consumption === null) consumption = 0;
        consumption *= -1;

        service.begin(fronius.getSiteConsumptionChart(service, 'fronius_' + service.name + '.consumption'));
        service.set(fronius.consumptionLoadId, Math.round(consumption));
        service.end();

        // Site Autonomy Chart
        service.begin(fronius.getSiteAutonomyChart(service, 'fronius_' + service.name + '.autonomy'));
        service.set(fronius.autonomyId, Math.round(site.rel_Autonomy));
        service.set(fronius.consumptionSelfId, Math.round(site.rel_SelfConsumption));
        service.end();

        // Inverters
        var inverters = json.Body.Data.Inverters;
        var inverterCount = Object.keys(inverters).length;
        if (inverterCount <= 0) return;
        var i = 1;
        for (i; i <= inverterCount; i++) {
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
