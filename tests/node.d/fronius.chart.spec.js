"use strict";

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "fronius",
    module: this
});

describe("fronius chart creation", function () {

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    it("should return a basic chart dimension", function () {
        var result = subject.createBasicDimension("id", "name", 2);

        expect(result.divisor).toBe(2);
        expect(result.id).toBe("id");
        expect(result.algorithm).toEqual(netdata.chartAlgorithms.absolute);
        expect(result.multiplier).toBe(1);
    });

    it("should return the power chart definition", function () {
        var id = "power";
        var result = subject.getSitePowerChart(service, id);

        expect(result.id).toBe(id);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("power");
        expect(result.context).toBe("fronius.power");
        expect(result.dimensions[subject.powerGridId].name).toBe("Grid");
        expect(result.dimensions[subject.powerPvId].name).toBe("Photovoltaics");
        expect(result.dimensions[subject.powerAccuId].name).toBe("Accumulator");
        expect(Object.keys(result.dimensions).length).toBe(3);
    });

    it("should return the consumption chart definition", function () {
        var id = "Load";
        var result = subject.getSiteConsumptionChart(service, id);

        expect(result.id).toBe(id);
        expect(result.units).toBe("W");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("consumption");
        expect(result.context).toBe("fronius.consumption");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.consumptionLoadId].name).toBe("Load");
    });

    it("should return the autonomy chart definition", function () {
        var id = "Autonomy";
        var result = subject.getSiteAutonomyChart(service, id);

        expect(result.id).toBe(id);
        expect(result.units).toBe("%");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("autonomy");
        expect(result.context).toBe("fronius.autonomy");
        expect(Object.keys(result.dimensions).length).toBe(2);
        expect(result.dimensions[subject.autonomyId].name).toBe("Autonomy");
        expect(result.dimensions[subject.consumptionSelfId].name).toBe("Self Consumption");
    });

    it("should return the energy today chart definition", function () {
        var id = "Energy today";
        var result = subject.getSiteEnergyTodayChart(service, id);

        expect(result.id).toBe(id);
        expect(result.units).toBe("kWh");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("energy");
        expect(result.context).toBe("fronius.energy.today");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.energyTodayId].name).toBe("Today");
    });

    it("should return the energy year chart definition", function () {
        var id = "Energy year";
        var result = subject.getSiteEnergyYearChart(service, id);

        expect(result.id).toBe(id);
        expect(result.units).toBe("kWh");
        expect(result.type).toBe(netdata.chartTypes.area);
        expect(result.family).toBe("energy");
        expect(result.context).toBe("fronius.energy.year");
        expect(Object.keys(result.dimensions).length).toBe(1);
        expect(result.dimensions[subject.energyYearId].name).toBe("Year");
    });

    it("should return the same chart definition on second call for lazy loading", function () {
        var first = subject.getSitePowerChart(service, "id");
        var second = subject.getSitePowerChart(service, "id");

        expect(first).toBe(second);
    });
});