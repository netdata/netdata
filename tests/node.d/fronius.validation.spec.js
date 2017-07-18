"use strict";

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "fronius",
    module: this
});

describe("fronius response validation", function () {

    // this is a faked JSON response from the server.
    // Used with freeformatter.com/json-escape.html to escape the json and turn it into a string.
    var fakeResponse = "{\r\n\t\"Head\" : {\r\n\t\t\"RequestArguments\" : {},\r\n\t\t\"Status\" " +
        ": {\r\n\t\t\t\"Code\" : 0,\r\n\t\t\t\"Reason\" : \"\",\r\n\t\t\t\"UserMessage\" : " +
        "\"\"\r\n\t\t},\r\n\t\t\"Timestamp\" : \"2017-07-17T16:01:04+02:00\"\r\n\t},\r\n\t\"Body\" : " +
        "{\r\n\t\t\"Data\" : {\r\n\t\t\t\"Site\" : {\r\n\t\t\t\t\"Mode\" : \"meter\",\r\n\t\t\t\t\"P_Grid\" " +
        ": -3430.729923,\r\n\t\t\t\t\"P_Load\" : -910.270077,\r\n\t\t\t\t\"P_Akku\" : " +
        "null,\r\n\t\t\t\t\"P_PV\" : 4341,\r\n\t\t\t\t\"rel_SelfConsumption\" : " +
        "20.969133,\r\n\t\t\t\t\"rel_Autonomy\" : 100,\r\n\t\t\t\t\"E_Day\" : 57230,\r\n\t\t\t\t\"E_Year\" " +
        ": 6425915.5,\r\n\t\t\t\t\"E_Total\" : 15388710,\r\n\t\t\t\t\"Meter_Location\" : " +
        "\"grid\"\r\n\t\t\t},\r\n\t\t\t\"Inverters\" : {\r\n\t\t\t\t\"1\" : {\r\n\t\t\t\t\t\"DT\" : " +
        "123,\r\n\t\t\t\t\t\"P\" : 4341,\r\n\t\t\t\t\t\"E_Day\" : 57230,\r\n\t\t\t\t\t\"E_Year\" : " +
        "6425915.5,\r\n\t\t\t\t\t\"E_Total\" : 15388710\r\n\t\t\t\t}\r\n\t\t\t}\r\n\t\t}\r\n\t}\r\n}";

    it("should do nothing if response is null", function () {
        netdata.send = jasmine.createSpy("send");

        subject.processResponse(service, null);
        var result = netdata.send.calls.count();

        expect(result).toBe(0);
    });

    it("should return null if response is null", function () {
        var result = subject.parseResponse(null);

        expect(result).toBeNull();
    });

    it("should return true if response is valid", function () {
        var result = subject.isResponseValid({
            "Body": {
                "Data": {
                    "Site": {
                        "Mode": "meter"
                    },
                    "Inverters": {
                        "1": {}
                    }
                }
            }
        });

        expect(result).toBeTruthy();
    });

    it("should return false if response is missing data", function () {
        var result = subject.isResponseValid({
            "Body": {}
        });

        expect(result).toBeFalsy();
    });

    it("should return false if response is missing inverter", function () {
        var result = subject.isResponseValid({
            "Body": {
                "Data": {
                    "Site": {}
                }
            }
        });

        expect(result).toBeFalsy();
    });

    it("should return false if response is missing inverter", function () {
        var result = subject.isResponseValid({
            "Body": {
                "Data": {
                    "Inverters": {}
                }
            }
        });

        expect(result).toBeFalsy();
    });

});

describe("fronius configuration validation", function () {

    it("should return 0 if there are no servers configured", function () {
        var result = subject.configure({});

        expect(result).toBe(0);
    });

    it("should return 0 if the servers array is empty", function () {
        var result = subject.configure({
            "servers": []
        });

        expect(result).toBe(0);
    });

    it("should return 0 if there is one server configured incorrectly", function () {
        var result = subject.configure({
            "servers": [{}]
        });

        expect(result).toBe(0);
    });

    it("should return 1 if there is one server configured", function () {
        subject.serviceExecute = jasmine.createSpy("serviceExecute");
        var name = "solar1";
        var result = subject.configure({
            "servers": [{
                "name": name,
                "api_path": "/api/",
                "hostname": "solar1.local"
            }]
        });

        expect(result).toBe(1);
        expect(subject.serviceExecute).toHaveBeenCalledWith(name, "solar1.local/api/", 5);
    });

    it("should return 2 if there are two servers configured", function () {
        subject.serviceExecute = jasmine.createSpy("serviceExecute");
        var name1 = "solar 1";
        var name2 = "solar 2";
        var result = subject.configure({
            "servers": [
                {
                    "name": name1,
                    "api_path": "/",
                    "hostname": "solar1.local"
                },
                {
                    "name": name2,
                    "api_path": "/",
                    "hostname": "solar2.local",
                    "update_every": 3
                }
            ]
        });

        expect(result).toBe(2);
        expect(subject.serviceExecute).toHaveBeenCalledWith(name1, "solar1.local/", 5);
        expect(subject.serviceExecute).toHaveBeenCalledWith(name2, "solar2.local/", 3);
    });

});