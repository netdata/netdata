"use strict";

var netdata = require("../../node.d/node_modules/netdata");
// remember: subject will be a singleton!
var subject = require("../../node.d/fronius.node");

var service = netdata.service({
    name: "validation",
    module: this
});

describe("fronius response validation", function () {

    it("should do nothing if response is null", function () {
        netdata.send = jasmine.createSpy("send");

        subject.processResponse(service, null);
        var result = netdata.send.calls.count();

        expect(result).toBe(0);
    });

    it("should return null if response is null", function () {
        var result = subject.convertToJson(null);

        expect(result).toBeNull();
    });

    it("should return null and log error if response cannot be parsed", function () {
        netdata.error = jasmine.createSpy("error");

        // trailing commas are enough to create syntax exceptions
        var result = subject.convertToJson("{name,}");

        expect(result).toBeNull();
        expect(netdata.error.calls.count()).toBe(1);
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