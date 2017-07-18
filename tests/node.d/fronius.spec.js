"use strict";
// delete these comments if not needed anymore.

var netdata = require("../../node.d/node_modules/netdata");
var fronius = require("../../node.d/fronius.node");

describe("fronius chart creation", function () {

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    it("should return a basic chart definition", function () {
        // act
        var result = fronius.createBasicDimension("id", "name", 2);
        // assert
        expect(result.divisor).toBe(2);
        expect(result.id).toBe("id");
        expect(result.algorithm).toEqual("absolute");
        expect(result.multiplier).toBe(1);
    });

    it("will fail", function () {
        netdata.debug("test");

        throw new Error("demonstrate failure of unit test runner");
    });

});

describe("fronius data parsing", function () {

    var service = netdata.service({
        name: "fronius",
        module: this
    });

    // this is a faked JSON response from the server.
    // Used with freeformatter.com/json-escape.html to escape the json and turn it into a string.
    var fakeResponse = "{\r\n\t\"Head\" : {\r\n\t\t\"RequestArguments\" : {},\r\n\t\t\"Status\" : {\r\n\t\t\t\"Code\" : 0,\r\n\t\t\t\"Reason\" : \"\",\r\n\t\t\t\"UserMessage\" : \"\"\r\n\t\t},\r\n\t\t\"Timestamp\" : \"2017-07-17T16:01:04+02:00\"\r\n\t},\r\n\t\"Body\" : {\r\n\t\t\"Data\" : {\r\n\t\t\t\"Site\" : {\r\n\t\t\t\t\"Mode\" : \"meter\",\r\n\t\t\t\t\"P_Grid\" : -3430.729923,\r\n\t\t\t\t\"P_Load\" : -910.270077,\r\n\t\t\t\t\"P_Akku\" : null,\r\n\t\t\t\t\"P_PV\" : 4341,\r\n\t\t\t\t\"rel_SelfConsumption\" : 20.969133,\r\n\t\t\t\t\"rel_Autonomy\" : 100,\r\n\t\t\t\t\"E_Day\" : 57230,\r\n\t\t\t\t\"E_Year\" : 6425915.5,\r\n\t\t\t\t\"E_Total\" : 15388710,\r\n\t\t\t\t\"Meter_Location\" : \"grid\"\r\n\t\t\t},\r\n\t\t\t\"Inverters\" : {\r\n\t\t\t\t\"1\" : {\r\n\t\t\t\t\t\"DT\" : 123,\r\n\t\t\t\t\t\"P\" : 4341,\r\n\t\t\t\t\t\"E_Day\" : 57230,\r\n\t\t\t\t\t\"E_Year\" : 6425915.5,\r\n\t\t\t\t\t\"E_Total\" : 15388710\r\n\t\t\t\t}\r\n\t\t\t}\r\n\t\t}\r\n\t}\r\n}"

    beforeAll(function () {
        // change this to enable debug log
        netdata.options.DEBUG = false;
    });

    it("should return a parsed value", function () {
        // arrange
        netdata.send = jasmine.createSpy("send");
        // act
        fronius.processResponse(service, fakeResponse);
        var result = netdata.send.calls.argsFor(0)[0];
        // assert
        expect(result).toContain("SET p_grid = -3431");
    });

});
