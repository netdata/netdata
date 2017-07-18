// delete these comments if not needed anymore.

function createState(min, max) {
    // create a fake state with only needed properties? Spying? not figured it out yet...
    return {
        tmp: {
            easyPieChartMin: min,
            easyPieChartMax: max
        }
    };
}


describe("percentage calculations for easy pie charts", function () {

    // some easy functions to test. incomplete yet.
    it('should return 50 if positive value between min and max', function () {
        // act
        var result = NETDATA.easypiechartPercentFromValueMinMax(createState(0, 0), 1, 0, 2);
        // assert
        expect(result).toBe(50);
    });

    it('should return 0.1 if value is zero', function () {
        // act
        var result = NETDATA.easypiechartPercentFromValueMinMax(createState(0, 0), 0, 0, 2);
        // assert
        expect(result).toBe(0.1);
    });

});


// with xdescribe, this is skipped.
// Delete the x to enable again and let it fail to test the build
describe('creation of easy pie charts', function () {

    beforeAll(function () {
        // karma stores the loaded files relative to "base/".
        // This command is needed to load HTML fixtures
        jasmine.getFixtures().fixturesPath = "base/tests/web/fixtures";
    });

    it('should create new chart', function () {
        // arrange
        // Theoretically we can load some html. What about jquery? could this work?
        // https://stackoverflow.com/questions/5337481/spying-on-jquery-selectors-in-jasmine
        loadFixtures("easypiechart.creation.fixture1.html");

        // for easy pie chart, we can fake the data result:
        var data = {
            result: [5]
        };
        // act
        var result = NETDATA.easypiechartChartCreate(createState(), data);
        // assert
        expect(result).toBe(true);
    });

});