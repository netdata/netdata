"use strict";
// SPDX-License-Identifier: GPL-3.0+

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "process",
    module: this
});

var exampleResponse = {
    "Body": {
        "Data": {
            "Site": {
                "Mode": "meter",
                "P_Grid": -3430.729923,
                "P_Load": -910.270077,
                "P_Akku": null,
                "P_PV": 4341,
                "rel_SelfConsumption": 20.969133,
                "rel_Autonomy": 100,
                "E_Day": 57230,
                "E_Year": 6425915.5,
                "E_Total": 15388710,
                "Meter_Location": "grid"
            },
            "Inverters": {
                "1": {
                    "DT": 123,
                    "P": 4341,
                    "E_Day": 57230,
                    "E_Year": 6425915.5,
                    "E_Total": 15388710
                }
            }
        }
    }
};

describe("fronius main processing", function () {

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    beforeEach(function () {
        deleteProperties(subject.charts);
    });

    it("should send parsed values to netdata", function () {
        netdata.send = jasmine.createSpy("send");

        subject.processResponse(service, exampleResponse);

        expect(netdata.send.calls.count()).toBe(6);

        // check if some parsed values were sent.
        var powerChart = netdata.send.calls.argsFor(5)[0];

        expect(powerChart).toContain("SET p_grid = -3431");
        expect(powerChart).toContain("SET p_pv = 4341");

        var inverterChart = netdata.send.calls.argsFor(0)[0];

        expect(inverterChart).toContain("SET 1 = 4341");

        var autonomyChart = netdata.send.calls.argsFor(3)[0];
        expect(autonomyChart).toContain("SET rel_selfconsumption = 21");
    });


});
