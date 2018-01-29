"use strict";

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "parse",
    module: this
});

var root = {
    "Body": {
        "Data": {
            "Site": {},
            "Inverters": {}
        }
    }
};

describe("fronius parsing for power chart", function () {

    var site = root.Body.Data.Site;

    afterEach(function () {
        deleteProperties(site);
    });

    it("should return 3000 for P_Grid when rounded", function () {
        site.P_Grid = 2999.501;
        var result = subject.parsePowerChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.powerGridId);
        expect(result.value).toBe(3000);
    });

    it("should return -3000 for P_Grid", function () {
        site.P_Grid = -3000;
        var result = subject.parsePowerChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.powerGridId);
        expect(result.value).toBe(-3000);
    });

    it("should return 0 for P_Grid if it is null", function () {
        site.P_Grid = null;
        var result = subject.parsePowerChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.powerGridId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for P_Grid if it is zero", function () {
        site.P_Grid = 0;
        var result = subject.parsePowerChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.powerGridId);
        expect(result.value).toBe(0);
    });

    it("should return -100 for P_Akku", function () {
        // it is unclear whether negative values are possible for p_akku (couln't test, nor any API docs found).
        site.P_Akku = -100;
        var result = subject.parsePowerChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.powerAccuId);
        expect(result.value).toBe(-100);
    });

    it("should return 0 for P_Akku if it is null", function () {
        site.P_Akku = null;
        var result = subject.parsePowerChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.powerAccuId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for P_Akku if it is zero", function () {
        site.P_Akku = 0;
        var result = subject.parsePowerChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.powerAccuId);
        expect(result.value).toBe(0);
    });

    it("should return 100 for P_PV", function () {
        site.P_PV = 100;
        var result = subject.parsePowerChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.powerPvId);
        expect(result.value).toBe(100);
    });

    it("should return 0 for P_PV if it is zero", function () {
        site.P_PV = 0;
        var result = subject.parsePowerChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.powerPvId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for P_PV if it is null", function () {
        site.P_PV = null;
        var result = subject.parsePowerChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.powerPvId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for P_PV if it is negative", function () {
        // solar panels shouldn't consume anything, only produce.
        site.P_PV = -1;
        var result = subject.parsePowerChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.powerPvId);
        expect(result.value).toBe(0);
    });

});

describe("fronius parsing for consumption", function () {

    var site = root.Body.Data.Site;

    afterEach(function () {
        deleteProperties(site);
    });

    it("should return 1000 for P_Load when rounded", function () {
        site.P_Load = 1000.499;
        var result = subject.parseConsumptionChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.consumptionLoadId);
        expect(result.value).toBe(1000);
    });

    it("should return absolute value for P_Load when negative", function () {
        /*
         with firmware 3.7.4 it is sometimes possible that negative values are returned for P_Load,
         which makes absolutely no sense. There is always a device that consumes some electricity around the clock.
         Best we can do is to make it a positive value, since 0 also doesn't make much sense.
         This "workaround" seems to work, as there couldn't be any strange peaks observed during long-time testing.
         */
        site.P_Load = -50;
        var result = subject.parseConsumptionChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.consumptionLoadId);
        expect(result.value).toBe(50);
    });

    it("should return 0 for P_Load if it is null", function () {
        site.P_Load = null;
        var result = subject.parseConsumptionChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.consumptionLoadId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for P_Load if it is zero", function () {
        site.P_Load = 0;
        var result = subject.parseConsumptionChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.consumptionLoadId);
        expect(result.value).toBe(0);
    });

});

describe("fronius parsing for autonomy", function () {

    var site = root.Body.Data.Site;

    afterEach(function () {
        deleteProperties(site);
    });

    it("should return 100 for rel_Autonomy", function () {
        site.rel_Autonomy = 100;
        var result = subject.parseAutonomyChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.autonomyId);
        expect(result.value).toBe(100);
    });

    it("should return 0 for rel_Autonomy if it is zero", function () {
        site.rel_Autonomy = 0;
        var result = subject.parseAutonomyChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.autonomyId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for rel_Autonomy if it is null", function () {
        site.rel_Autonomy = null;
        var result = subject.parseAutonomyChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.autonomyId);
        expect(result.value).toBe(0);
    });

    it("should return 20 for rel_Autonomy if it is 20", function () {
        site.rel_Autonomy = 20.1;
        var result = subject.parseAutonomyChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.autonomyId);
        expect(result.value).toBe(20);
    });

    it("should return 20 for rel_SelfConsumption if it is 19.5", function () {
        site.rel_SelfConsumption = 19.5;
        var result = subject.parseAutonomyChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.consumptionSelfId);
        expect(result.value).toBe(20);
    });

    it("should return 100 for rel_SelfConsumption if it is null", function () {
        /*
         During testing it could be observed that the API is delivering null if the solar panels
         do not produce enough energy to supply the local load. But in this case it should be 100, since all
         the produced energy is directly consumed.
         */
        site.rel_SelfConsumption = null;
        var result = subject.parseAutonomyChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.consumptionSelfId);
        expect(result.value).toBe(100);
    });

    it("should return 0 for rel_SelfConsumption if it is zero", function () {
        site.rel_SelfConsumption = 0;
        var result = subject.parseAutonomyChart(service, site).dimensions[1];

        expect(result.name).toBe(subject.consumptionSelfId);
        expect(result.value).toBe(0);
    });

    it("should return 0 for solarConsumption if PV is null", function () {
        site.P_PV = null;
        site.P_Load = -1000;
        var result = subject.parseAutonomyChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.solarConsumptionId);
        expect(result.value).toBe(0);
    });

    it("should return 100 for solarConsumption if Load is higher than solar power", function () {
        site.P_PV = 500;
        site.P_Load = -1500;
        var result = subject.parseAutonomyChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.solarConsumptionId);
        expect(result.value).toBe(100);
    });

    it("should return 50 for solarConsumption if Load is half than solar power", function () {
        site.P_PV = 3000;
        site.P_Load = -1500;
        var result = subject.parseAutonomyChart(service, site).dimensions[2];

        expect(result.name).toBe(subject.solarConsumptionId);
        expect(result.value).toBe(50);
    });
});

describe("fronius parsing for energy", function () {

    var site = root.Body.Data.Site;

    afterEach(function () {
        deleteProperties(site);
    });

    it("should return 10000 for E_Day", function () {
        site.E_Day = 10000;
        var result = subject.parseEnergyTodayChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.energyTodayId);
        expect(result.value).toBe(10000);
    });

    it("should return 0 for E_Day if it is negative", function () {
        /*
         The solar panels can't produce negative energy, really. It would be a fault of the API.
         */
        site.E_Day = -0.4;
        var result = subject.parseEnergyTodayChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.energyTodayId);
        expect(result.value).toBe(0);
    });

    it("should return 100'000 for E_Year", function () {
        site.E_Year = 100000.4;
        var result = subject.parseEnergyYearChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.energyYearId);
        expect(result.value).toBe(100000);
    });

    it("should return 0 for E_Year if it is negative", function () {
        /*
         A return value of 0 only makes sense in the silvester night anyway, when the counter is being reset.
         A negative value is a fault from the API though. It wouldn't make sense.
         */
        site.E_Year = -1;
        var result = subject.parseEnergyYearChart(service, site).dimensions[0];

        expect(result.name).toBe(subject.energyYearId);
        expect(result.value).toBe(0);
    });
});

describe("fronius parsing for inverters", function () {

    var inverters = root.Body.Data.Inverters;

    afterEach(function () {
        deleteProperties(inverters);
    });

    it("should return 1000 for P for inverter with name", function () {
       inverters["cellar"] = {
           P: 1000
       };
       var result = subject.parseInverterChart(service, inverters).dimensions[0];

       expect(result.name).toBe("cellar");
       expect(result.value).toBe(1000);
    });

});