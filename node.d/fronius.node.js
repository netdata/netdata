"use strict";

// This program will connect to one or more Fronius Symo Inverters.
// to get the Solar Power Generated (current, today).

// example configuration in netdata/conf.d/node.d/fronius.conf.md

var url = require("url");
var http = require("http");
var netdata = require("netdata");

netdata.debug("loaded " + __filename + " plugin");

var fronius = {
    name: "Fronius",
    enable_autodetect: false,
    update_every: 5,
    base_priority: 60000,
    charts: {},

    powerGridId: "p_grid",
    powerPvId: "p_pv",
    powerAccuId: "p_akku", // not my typo! Using the ID from the AP
    consumptionLoadId: "p_load",
    autonomyId: "rel_autonomy",
    consumptionSelfId: "rel_selfconsumption",
    energyTodayId: "e_day",
    energyYearId: "e_year",

    createBasicDimension: function (id, name, divisor) {
        return {
            id: id,                                     // the unique id of the dimension
            name: name,                                 // the name of the dimension
            algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
            multiplier: 1,                              // the multiplier
            divisor: divisor,                           // the divisor
            hidden: false                               // is hidden (boolean)
        };
    },

    // Gets the site power chart. Will be created if not existing.
    getSitePowerChart: function (service, suffix) {
        var id = this.getChartId(service, suffix);
        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};
        dim[fronius.powerGridId] = this.createBasicDimension(fronius.powerGridId, "grid", 1);
        dim[fronius.powerPvId] = this.createBasicDimension(fronius.powerPvId, "photovoltaics", 1);
        dim[fronius.powerAccuId] = this.createBasicDimension(fronius.powerAccuId, "accumulator", 1);

        chart = {
            id: id,                                         // the unique id of the chart
            name: "",                                       // the unique name of the chart
            title: service.name + " Current Site Power",    // the title of the chart
            units: "W",                                     // the units of the chart dimensions
            family: "power",                                // the family of the chart
            context: "fronius.power",                       // the context of the chart
            type: netdata.chartTypes.area,                  // the type of the chart
            priority: fronius.base_priority + 1,            // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(id, chart);
        fronius.charts[id] = chart;

        return chart;
    },

    // Gets the site consumption chart. Will be created if not existing.
    getSiteConsumptionChart: function (service, suffix) {
        var id = this.getChartId(service, suffix);
        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[fronius.consumptionLoadId] = this.createBasicDimension(fronius.consumptionLoadId, "load", 1);

        chart = {
            id: id,                                         // the unique id of the chart
            name: "",                                       // the unique name of the chart
            title: service.name + " Current Load",          // the title of the chart
            units: "W",                                     // the units of the chart dimensions
            family: "consumption",                          // the family of the chart
            context: "fronius.consumption",                 // the context of the chart
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
    getSiteAutonomyChart: function (service, suffix) {
        var id = this.getChartId(service, suffix);
        var chart = fronius.charts[id];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[fronius.autonomyId] = this.createBasicDimension(fronius.autonomyId, "autonomy", 1);
        dim[fronius.consumptionSelfId] = this.createBasicDimension(fronius.consumptionSelfId, "self_consumption", 1);

        chart = {
            id: id,                                         // the unique id of the chart
            name: "",                                       // the unique name of the chart
            title: service.name + " Current Autonomy",      // the title of the chart
            units: "%",                                     // the units of the chart dimensions
            family: "autonomy",                             // the family of the chart
            context: "fronius.autonomy",                    // the context of the chart
            type: netdata.chartTypes.area,                  // the type of the chart
            priority: fronius.base_priority + 3,            // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(id, chart);
        fronius.charts[id] = chart;

        return chart;
    },

    // Gets the site energy chart for today. Will be created if not existing.
    getSiteEnergyTodayChart: function (service, suffix) {
        var chartId = this.getChartId(service, suffix);
        var chart = fronius.charts[chartId];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[fronius.energyTodayId] = this.createBasicDimension(fronius.energyTodayId, "today", 1000);
        chart = {
            id: chartId,                                         // the unique id of the chart
            name: "",                                            // the unique name of the chart
            title: service.name + " Energy production for today",// the title of the chart
            units: "kWh",                                        // the units of the chart dimensions
            family: "energy",                                    // the family of the chart
            context: "fronius.energy.today",                     // the context of the chart
            type: netdata.chartTypes.area,                       // the type of the chart
            priority: fronius.base_priority + 4,                 // the priority relative to others in the same family
            update_every: service.update_every,                  // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        fronius.charts[chartId] = chart;

        return chart;
    },

    // Gets the site energy chart for today. Will be created if not existing.
    getSiteEnergyYearChart: function (service, suffix) {
        var chartId = this.getChartId(service, suffix);
        var chart = fronius.charts[chartId];
        if (fronius.isDefined(chart)) return chart;
        var dim = {};
        dim[fronius.energyYearId] = this.createBasicDimension(fronius.energyYearId, "year", 1000);
        chart = {
            id: chartId,                                             // the unique id of the chart
            name: "",                                                // the unique name of the chart
            title: service.name + " Energy production for this year",// the title of the chart
            units: "kWh",                                            // the units of the chart dimensions
            family: "energy",                                        // the family of the chart
            context: "fronius.energy.year",                          // the context of the chart
            type: netdata.chartTypes.area,                           // the type of the chart
            priority: fronius.base_priority + 5,                     // the priority relative to others in the same family
            update_every: service.update_every,                      // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        fronius.charts[chartId] = chart;

        return chart;
    },

    // Gets the inverter power chart. Will be created if not existing.
    // Needs the array of inverters in order to create a chart with all inverters as dimensions
    getInverterPowerChart: function (service, suffix, inverters) {
        var chartId = this.getChartId(service, suffix);
        var chart = fronius.charts[chartId];
        if (fronius.isDefined(chart)) return chart;

        var dim = {};
        for (var key in inverters) {
            if (inverters.hasOwnProperty(key)) {
                var name = key;
                if (!isNaN(key)) name = "inverter_" + key;
                dim[key] = this.createBasicDimension("inverter_" + key, name, 1);
            }
        }

        chart = {
            id: chartId,                                     // the unique id of the chart
            name: "",                                        // the unique name of the chart
            title: service.name + " Current Inverter Output",// the title of the chart
            units: "W",                                      // the units of the chart dimensions
            family: "inverters",                             // the family of the chart
            context: "fronius.inverter.output",              // the context of the chart
            type: netdata.chartTypes.stacked,                // the type of the chart
            priority: fronius.base_priority + 6,             // the priority relative to others in the same family
            update_every: service.update_every,              // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        fronius.charts[chartId] = chart;

        return chart;
    },

    processResponse: function (service, content) {
        var json = fronius.convertToJson(content);
        if (json === null) return;

        // add the service
        service.commit();

        var chartDefinitions = fronius.parseCharts(service, json);
        var chartCount = chartDefinitions.length;
        while (chartCount--) {
            var chartObj = chartDefinitions[chartCount];
            service.begin(chartObj.chart);
            var dimCount = chartObj.dimensions.length;
            while (dimCount--) {
                var dim = chartObj.dimensions[dimCount];
                service.set(dim.name, dim.value);
            }
            service.end();
        }
    },

    parseCharts: function (service, json) {
        var site = json.Body.Data.Site;
        return [
            this.parsePowerChart(service, site),
            this.parseConsumptionChart(service, site),
            this.parseAutonomyChart(service, site),
            this.parseEnergyTodayChart(service, site),
            this.parseEnergyYearChart(service, site),
            this.parseInverterChart(service, json.Body.Data.Inverters)
        ];
    },

    parsePowerChart: function (service, site) {
        return this.getChart(this.getSitePowerChart(service, "power"),
            [
                this.getDimension(this.powerGridId, Math.round(site.P_Grid)),
                this.getDimension(this.powerPvId, Math.round(Math.max(site.P_PV, 0))),
                this.getDimension(this.powerAccuId, Math.round(site.P_Akku))
            ]
        );
    },

    parseConsumptionChart: function (service, site) {
        return this.getChart(this.getSiteConsumptionChart(service, "consumption"),
            [this.getDimension(this.consumptionLoadId, Math.round(Math.abs(site.P_Load)))]
        );
    },

    parseAutonomyChart: function (service, site) {
        var selfConsumption = site.rel_SelfConsumption;
        return this.getChart(this.getSiteAutonomyChart(service, "autonomy"),
            [
                this.getDimension(this.autonomyId, Math.round(site.rel_Autonomy)),
                this.getDimension(this.consumptionSelfId, Math.round(selfConsumption === null ? 100 : selfConsumption))
            ]
        );
    },

    parseEnergyTodayChart: function (service, site) {
        return this.getChart(this.getSiteEnergyTodayChart(service, "energy.today"),
            [this.getDimension(this.energyTodayId, Math.round(Math.max(site.E_Day, 0)))]
        );
    },

    parseEnergyYearChart: function (service, site) {
        return this.getChart(this.getSiteEnergyYearChart(service, "energy.year"),
            [this.getDimension(this.energyYearId, Math.round(Math.max(site.E_Year, 0)))]
        );
    },

    parseInverterChart: function (service, inverters) {
        var dimensions = [];
        for (var key in inverters) {
            if (inverters.hasOwnProperty(key)) {
                dimensions.push(this.getDimension(key, Math.round(inverters[key].P)));
            }
        }
        return this.getChart(this.getInverterPowerChart(service, "inverters.output", inverters), dimensions);
    },

    getDimension: function (name, value) {
        return {
            name: name,
            value: value
        };
    },

    getChart: function (chart, dimensions) {
        return {
            chart: chart,
            dimensions: dimensions
        };
    },

    getChartId: function (service, suffix) {
        return "fronius_" + service.name + "." + suffix;
    },

    convertToJson: function (httpBody) {
        if (httpBody === null) return null;
        var json = httpBody;
        // can't parse if it's already a json object,
        // the check enables easier testing if the httpBody is already valid JSON.
        if (typeof httpBody !== "object") {
            try {
                json = JSON.parse(httpBody);
            } catch (error) {
                netdata.error("fronius: Got a response, but it is not valid JSON. Ignoring. Error: " + error.message);
                return null;
            }
        }
        return this.isResponseValid(json) ? json : null;
    },

    // some basic validation
    isResponseValid: function (json) {
        if (this.isUndefined(json.Body)) return false;
        if (this.isUndefined(json.Body.Data)) return false;
        if (this.isUndefined(json.Body.Data.Site)) return false;
        return this.isDefined(json.Body.Data.Inverters);
    },

    // module.serviceExecute()
    // this function is called only from this module
    // its purpose is to prepare the request and call
    // netdata.serviceExecute()
    serviceExecute: function (name, uri, update_every) {
        netdata.debug(this.name + ": " + name + ": url: " + uri + ", update_every: " + update_every);

        var service = netdata.service({
            name: name,
            request: netdata.requestFromURL("http://" + uri),
            update_every: update_every,
            module: this
        });
        service.execute(this.processResponse);
    },


    configure: function (config) {
        if (fronius.isUndefined(config.servers)) return 0;
        var added = 0;
        var len = config.servers.length;
        while (len--) {
            var server = config.servers[len];
            if (fronius.isUndefined(server.update_every)) server.update_every = this.update_every;
            if (fronius.areUndefined([server.name, server.hostname, server.api_path])) continue;

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
        return typeof value === "undefined";
    },

    areUndefined: function (valueArray) {
        var i = 0;
        for (i; i < valueArray.length; i++) {
            if (this.isUndefined(valueArray[i])) return true;
        }
        return false;
    },

    isDefined: function (value) {
        return typeof value !== "undefined";
    }
};

module.exports = fronius;
