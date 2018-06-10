'use strict';
// SPDX-License-Identifier: GPL-3.0+

// This program will connect to one Stiebel Eltron ISG for heatpump heating
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

    createBasicDimension: function (id, name, multiplier, divisor) {
        return {
            id: id,                                     // the unique id of the dimension
            name: name,                                 // the name of the dimension
            algorithm: netdata.chartAlgorithms.absolute,// the id of the netdata algorithm
            multiplier: multiplier,                     // the multiplier
            divisor: divisor,                           // the divisor
            hidden: false                               // is hidden (boolean)
        };
    },

    processResponse: function (service, html) {
        if (html === null) return;

        // add the service
        service.commit();

        var page = stiebeleltron.pages[service.name];
        var categories = page.categories;
        var categoriesCount = categories.length;
        while (categoriesCount--) {
            var context = {
                html: html,
                service: service,
                category: categories[categoriesCount],
                page: page,
                chartDefinition: null,
                dimension: null
            };
            stiebeleltron.processCategory(context);

        }
    },

    processCategory: function (context) {
        var charts = context.category.charts;
        var chartCount = charts.length;
        while (chartCount--) {
            context.chartDefinition = charts[chartCount];
            stiebeleltron.processChart(context);
        }
    },

    processChart: function (context) {
        var dimensions = context.chartDefinition.dimensions;
        var dimensionCount = dimensions.length;
        context.service.begin(stiebeleltron.getChartFromContext(context));

        while (dimensionCount--) {
            context.dimension = dimensions[dimensionCount];
            stiebeleltron.processDimension(context);
        }
        context.service.end();
    },

    processDimension: function (context) {
        var dimension = context.dimension;
        var match = new RegExp(dimension.regex).exec(context.html);
        if (match === null) return;
        var value = match[1].replace(",", ".");
        // most values have a single digit by default, which requires the values to be multiplied. can be overridden.
        if (stiebeleltron.isDefined(dimension.digits)) {
            value *= Math.pow(10, dimension.digits);
        } else {
            value *= 10;
        }
        context.service.set(stiebeleltron.getDimensionId(context), value);
    },

    getChartFromContext: function (context) {
        var chartId = this.getChartId(context);
        var chart = stiebeleltron.charts[chartId];
        if (stiebeleltron.isDefined(chart)) return chart;

        var chartDefinition = context.chartDefinition;
        var service = context.service;
        var dimensions = {};

        var dimCount = chartDefinition.dimensions.length;
        while (dimCount--) {
            var dim = chartDefinition.dimensions[dimCount];
            var multiplier = 1;
            var divisor = 10;
            if (stiebeleltron.isDefined(dim.digits)) divisor = Math.pow(10, Math.max(0, dim.digits));
            if (stiebeleltron.isDefined(dim.multiplier)) multiplier = dim.multiplier;
            if (stiebeleltron.isDefined(dim.divisor)) divisor = dim.divisor;
            context.dimension = dim;
            var dimId = this.getDimensionId(context);
            dimensions[dimId] = this.createBasicDimension(dimId, dim.name, multiplier, divisor);
        }

        chart = {
            id: chartId,
            name: '',
            title: chartDefinition.title,
            units: chartDefinition.unit,
            family: context.category.name,
            context: 'stiebeleltron.' + context.category.id + '.' + chartDefinition.id,
            type: chartDefinition.type,
            priority: stiebeleltron.base_priority + chartDefinition.prio,// the priority relative to others in the same family
            update_every: service.update_every,             // the expected update frequency of the chart
            dimensions: dimensions
        };
        chart = service.chart(chartId, chart);
        stiebeleltron.charts[chartId] = chart;

        return chart;
    },

    // module.serviceExecute()
    // this function is called only from this module
    // its purpose is to prepare the request and call
    // netdata.serviceExecute()
    serviceExecute: function (name, uri, update_every) {
        netdata.debug(this.name + ': ' + name + ': url: ' + uri + ', update_every: ' + update_every);

        var service = netdata.service({
            name: name,
            request: netdata.requestFromURL(uri),
            update_every: update_every,
            module: this
        });
        service.execute(this.processResponse);
    },


    configure: function (config) {
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
            this.pages[page.name] = page;
            this.serviceExecute(page.name, page.url, page.update_every);
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

    getChartId: function (context) {
        return "stiebeleltron_" + context.page.id +
            "." + context.category.id +
            "." + context.chartDefinition.id;
    },

    getDimensionId: function (context) {
        return context.dimension.id;
    },

    isUndefined: function (value) {
        return typeof value === 'undefined';
    },

    isDefined: function (value) {
        return typeof value !== 'undefined';
    }
};

module.exports = stiebeleltron;
