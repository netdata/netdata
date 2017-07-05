'use strict';

// This program will connect to one Stiebel Eltron ISG for heatpump heating.
// to get the heat pump metrics.

// example configuration in netdata/conf.d/node.d/stiebeleltron.conf.md

var url = require("url");
var http = require("http");
var netdata = require("netdata");

netdata.debug("loaded " + __filename + " plugin");

var stiebeleltron = {
    name: "Stiebel Eltron",
    enable_autodetect: false,
    update_every: 10,
    base_priority: 60000,
    charts: {},
    pages: {},

    createBasicDimension(id, name) {
        return {
            id: id,                                     // the unique id of the dimension
            name: name,                                 // the name of the dimension
            algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
            multiplier: 1,                              // the multiplier
            divisor: 1,                                 // the divisor
            hidden: false                               // is hidden (boolean)
        };
    },

    processResponse(service, html) {
        if (html === null) return;

        // add the service
        service.commit();

        var page = stiebeleltron.pages[service.name];
        var categories = page.categories;
        var categoriesCount = categories.length;
        while (categoriesCount--) {
            var category = categories[categoriesCount];
            var context = {
                html: html,
                service: service,
                category: category,
                page: page
            };
            stiebeleltron.processCategory(context);

        }
    },

    processCategory(context) {
        var charts = context.category.charts;
        var chartCount = charts.length;
        while (chartCount--) {
            var chart = charts[chartCount];
            context.chartDefinition = chart;
            stiebeleltron.processChart(context);
        }
    },


    processChart(context) {
        var dimensions = context.chartDefinition.dimensions;
        var dimensionCount = dimensions.length;
        context.service.begin(stiebeleltron.getChartUsing(context));

        while(dimensionCount--) {
            var dimension = dimensions[dimensionCount];
            stiebeleltron.processDimension(dimension, context);
        }
        context.service.end();
    },

    getChartUsing(context) {
        var chartId = "stiebeleltron_" + context.page.id +
            "." + context.category.id +
            "." + context.chartDefinition.id;
        var chart = stiebeleltron.charts[chartId];
        if (stiebeleltron.isDefined(chart)) return chart;

        var dim = {};

        var inverter_count = Object.keys(inverters).length;
        var inverter = inverters[inverter_count.toString()];
        var i = 1;
        for (i; i <= inverter_count; i++) {
            if (stiebeleltron.isUndefined(inverter)) {
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
            title: service.name + " " + chartDefinition.title,    // the title of the chart
            units: chartDefinition.unit,                                   // the units of the chart dimensions
            family: 'Inverters',                                  // the family of the chart
            context: 'stiebeleltron.inverter',                // the context of the chart
            type: netdata.chartTypes.stacked,                  // the type of the chart
            priority: stiebeleltron.base_priority + 5,             // the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dim
        };
        chart = service.chart(chartId, chart);
        stiebeleltron.charts[chartId] = chart;

        return chart;
    },

    processDimension(dimension, context) {
        var value = stiebeleltron.parseRegex(dimension.regex, context.html);
        context.service.set(dimension.name, Math.round(value));
    },

    parseRegex(regex, html) {

    },

    // module.serviceExecute()
    // this function is called only from this module
    // its purpose is to prepare the request and call
    // netdata.serviceExecute()
    serviceExecute(name, uri, update_every) {
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


    configure(config) {
        if (stiebeleltron.isUndefined(config.pages)) return 0;
        var added = 0;
        var pageCount = config.pages.length;
        while (pageCount--) {
            var page = config.pages[pageCount];
            // some validation
            if (stiebeleltron.isUndefined(page.categories) || page.categories.length < 1) {
                netdata.error("Your Stiebel Eltron config is invalid. Disabling plugin.");
                return 0;
            }
            if (stiebeleltron.isUndefined(page.update_every)) page.update_every = this.update_every;
            this.pages[page.id] = page;
            this.serviceExecute(page.name, page.url, page.update_every);
            added++;
        }
        return added;
    },

    // module.update()
    // this is called repeatedly to collect data, by calling
    // netdata.serviceExecute()
    update(service, callback) {
        service.execute(function (serv, data) {
            service.module.processResponse(serv, data);
            callback();
        });
    },

    isUndefined(value) {
        return typeof value === 'undefined';
    },

    isDefined(value) {
        return typeof value !== 'undefined';
    },
};

module.exports = stiebeleltron;
