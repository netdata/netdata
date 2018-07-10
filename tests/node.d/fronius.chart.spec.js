"use strict";
// SPDX-License-Identifier: GPL-3.0+

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "chart",
    module: this
});

describe("fronius chart creation", function () {

    var chartPrefix = "fronius_chart.";

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    afterAll(function () {
        deleteProperties(subject.charts)
    });

    it("should return a basic chart dimension", function () {
        var result = subject.createBasicDimension("id", "name", 2);

        expect(result.divisor).toBe(2);
        expect(result.id).toBe("id");
        expect(result.algorithm).toEqual(netdata.chartAlgorithms.absolute);
        expect(result.multiplier).toBe(1);
    });

    it("should return the power chart definition", function () {
        var suffix = "power";
        var result = subject.getSitePowerChart(service, suffix);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("power");
        expect(result.context).toBe("fronius.power");
        expect(result.dimensions[subject.powerGridId].name).toBe("grid");
        expect(result.dimensions[subject.powerPvId].name).toBe("photovoltaics");
        expect(result.dimensions[subject.powerAccuId].name).toBe("accumulator");
        expect(Object.keys(result.dimensions).length).toBe(3);
    });

    it("should return the consumption chart definition", function () {
        var suffix = "Load";
        var result = subject.getSiteConsumptionChart(service, suffix);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("consumption");
        expect(result.context).toBe("fronius.consumption");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.consumptionLoadId].name).toBe("load");
    });

    it("should return the autonomy chart definition", function () {
        var suffix = "Autonomy";
        var result = subject.getSiteAutonomyChart(service, suffix);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("%");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("autonomy");
        expect(result.context).toBe("fronius.autonomy");
        expect(Object.keys(result.dimensions).length).toBe(3);
        expect(result.dimensions[subject.autonomyId].name).toBe("autonomy");
        expect(result.dimensions[subject.consumptionSelfId].name).toBe("self_consumption");
    });

    it("should return the energy today chart definition", function () {
        var suffix = "Energy today";
        var result = subject.getSiteEnergyTodayChart(service, suffix);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("kWh");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("energy");
        expect(result.context).toBe("fronius.energy.today");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.energyTodayId].name).toBe("today");
    });

    it("should return the energy year chart definition", function () {
        var suffix = "Energy year";
        var result = subject.getSiteEnergyYearChart(service, suffix);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("kWh");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("energy");
        expect(result.context).toBe("fronius.energy.year");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.energyYearId].name).toBe("year");
    });

    it("should return the inverter chart definition with a single numerical inverter", function () {
        var inverters = {
            "1": {}
        };
        var suffix = "numerical";
        var result = subject.getInverterPowerChart(service, suffix, inverters);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.stacked);
        expect(result.family).toBe("inverters");
        expect(result.context).toBe("fronius.inverter.output");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions["1"].name).toBe("inverter_1");
    });

    it("should return the inverter chart definition with a single alphabetical inverter", function () {
        var key = "Cellar";
        var inverters = {
            "Cellar": {}
        };
        var suffix = "alphabetical";
        var result = subject.getInverterPowerChart(service, suffix, inverters);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.stacked);
        expect(result.family).toBe("inverters");
        expect(result.context).toBe("fronius.inverter.output");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[key].name).toBe(key);
    });

    it("should return the inverter chart definition with multiple alphanumerical inverter", function () {
        var alpha = "Cellar";
        var numerical = 1;
        var inverters = {
            "Cellar": {},
            "1": {}
        };
        var suffix = "alphanumerical";
        var result = subject.getInverterPowerChart(service, suffix, inverters);

        expect(result.id).toBe(chartPrefix + suffix);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.stacked);
        expect(result.family).toBe("inverters");
        expect(result.context).toBe("fronius.inverter.output");
        expect(Object.keys(result.dimensions).length).toBe(2);
        expect(result.dimensions[alpha].name).toBe(alpha);
        expect(result.dimensions[numerical].name).toBe("inverter_" + numerical);
    });

    it("should return the same chart definition on second call for lazy loading", function () {
        var first = subject.getSitePowerChart(service, "id");
        var second = subject.getSitePowerChart(service, "id");

        expect(first).toBe(second);
    });
});
