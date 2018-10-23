"use strict";


describe("percentage calculations for easy pie charts with dynamic range", function () {

    it("should return positive value, if value greater than dynamic max", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 6, 2, 10);

        expect(result).toBe(60);
    });

    it("should return negative value, if value lesser than dynamic min", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, -6, -10, 10);

        expect(result).toBe(-60);
    });

    it("should return 0 if value is zero and min negative, max positive", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 0, -1, 2);

        expect(result).toBe(0);
    });

    it("should return 0.1 if value and min are zero and max positive", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 0, 0, 2);

        expect(result).toBe(0.1);
    });

    it("should return -0.1 if value is zero, max and min negative", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 0, -2, -1);

        expect(result).toBe(-0.1);
    });

    it("should return positive value, if max is user-defined", function () {
        var state = createState(null, 50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 46, -40, 50);

        expect(result).toBe(92);
    });

    it("should return negative value, if min is user-defined", function () {
        var state = createState(-50, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, -46, -50, 40);

        expect(result).toBe(-92);
    });

});

describe("percentage calculations for easy pie charts with fixed range", function () {

    it("should return positive value, if min and max are user-defined", function () {
        var state = createState(40, 50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 46, 40, 50);

        expect(result).toBe(60);
    });

    it("should return 100 if positive min and max are user-defined, but value is greater than max", function () {
        var state = createState(40, 50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 60, 40, 50);

        expect(result).toBe(100);
    });

    it("should return 0.1 if positive min and max are user-defined, but value is smaller than min", function () {
        var state = createState(40, 50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 39.9, 42, 48);

        expect(result).toBe(0.1);
    });

    it("should return -100 if negative min and max are user-defined, but value is smaller than min", function () {
        var state = createState(-40, -50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, -50.1, -40, -50);

        expect(result).toBe(-100);
    });

    it("should return 0.1 if negative min and max are user-defined, but value is smaller than min", function () {
        var state = createState(-40, -50);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, -50.1, -20, -45);

        expect(result).toBe(-100);
    });
});

describe("percentage calculations for easy pie charts with invalid input", function () {

    it("should return 0.1 if value undefined", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, null, 40, 50);

        expect(result).toBe(0.1);
    });

    it("should return positive value if min is undefined", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 1, null, 2);

        expect(result).toBe(50);
    });

    it("should return positive if max is undefined", function () {
        var state = createState(null, null);

        var result = NETDATA.easypiechartPercentFromValueMinMax(state, 21, 42, null);

        expect(result).toBe(50);
    });
});

function createState(min, max) {
    // create a fake state with only the needed properties.
    return {
        tmp: {
            easyPieChartMin: min,
            easyPieChartMax: max
        }
    };
}
