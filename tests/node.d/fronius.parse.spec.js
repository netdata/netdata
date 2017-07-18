"use strict";

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "fronius",
    module: this
});

describe("fronius data parsing", function () {

    var fakeResponse = {
        "Head" : {
            "RequestArguments" : {},
            "Status" : {
                "Code" : 0,
                "Reason" : "",
                "UserMessage" : ""
            },
            "Timestamp" : "2017-07-17T16:01:04+02:00"
        },
        "Body" : {
            "Data" : {
                "Site" : {
                    "Mode" : "meter",
                    "P_Grid" : -3430.729923,
                    "P_Load" : -910.270077,
                    "P_Akku" : null,
                    "P_PV" : 4341,
                    "rel_SelfConsumption" : 20.969133,
                    "rel_Autonomy" : 100,
                    "E_Day" : 57230,
                    "E_Year" : 6425915.5,
                    "E_Total" : 15388710,
                    "Meter_Location" : "grid"
                },
                "Inverters" : {
                    "1" : {
                        "DT" : 123,
                        "P" : 4341,
                        "E_Day" : 57230,
                        "E_Year" : 6425915.5,
                        "E_Total" : 15388710
                    }
                }
            }
        }
    };

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    it("should return a parsed value", function () {
        // arrange
        netdata.send = jasmine.createSpy("send");
        // act
        subject.processResponse(service, fakeResponse);
        var result = netdata.send.calls.argsFor(0)[0];
        // assert
        expect(result).toContain("SET p_grid = -3431");
    });

});
