"use strict";


// with xdescribe, this is skipped.
describe("creation of easy pie charts", function () {

    beforeAll(function () {
        // karma stores the loaded files relative to "base/".
        // This command is needed to load HTML fixtures
        jasmine.getFixtures().fixturesPath = "base/tests/web/fixtures";
    });

    it("should create new chart, but it's failure is expected for demonstration purpose", function () {
        // arrange
        // Theoretically we can load some html. What about jquery? could this work?
        // https://stackoverflow.com/questions/5337481/spying-on-jquery-selectors-in-jasmine
        loadFixtures("easypiechart.chart.fixture1.html");

        // for easy pie chart, we can fake the data result:
        var data = {
            result: [5]
        };
        // act
        var result = NETDATA.easypiechartChartCreate(createState(), data);
        // assert
        expect(result).toBe(true);
    });

    function createState(min, max) {
        // create a fake state with only needed properties.
        return {
            tmp: {
                easyPieChartMin: min,
                easyPieChartMax: max
            }
        };
    }

});