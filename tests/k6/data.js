import http from "k6/http";
import { log, check, group, sleep } from "k6";
import { Rate } from "k6/metrics";

// A custom metric to track failure rates
var failureRate = new Rate("check_failure_rate");

// Options
export let options = {
    stages: [
        // Linearly ramp up from 1 to 20 VUs during first 30s
        { target: 20, duration: "30s" },
        // Hold at 50 VUs for the next 1 minute
        { target: 20, duration: "1m" },
        // Linearly ramp down from 50 to 0 VUs over the last 10 seconds
        { target: 0, duration: "10s" }
    ],
    thresholds: {
        // We want the 95th percentile of all HTTP request durations to be less than 500ms
        "http_req_duration": ["p(95)<500"],
        // Requests with the fast tag should finish even faster
        "http_req_duration{fast:yes}": ["p(99)<250"],
        // Thresholds based on the custom metric we defined and use to track application failures
        "check_failure_rate": [
            // Global failure rate should be less than 1%
            "rate<0.01",
            // Abort the test early if it climbs over 5%
            { threshold: "rate<=0.05", abortOnFail: true },
        ],
    },
};

function rnd(min, max) {
  min = Math.ceil(min);
  max = Math.floor(max);
  return Math.floor(Math.random() * (max - min)) + min; //The maximum is exclusive and the minimum is inclusive
}

// Main function
export default function () {
    // Control what the data request asks for
    let charts = [ "example.random" ]
    let chartmin = 0;
    let chartmax = charts.length - 1; 
    let aftermin = 60;
    let aftermax = 3600;
    let beforemin = 3503600;
    let beforemax = 3590000;
    let pointsmin = 300;
    let pointsmax = 3600;

    group("Requests", function () {
        // Execute multiple requests in parallel like a browser, to fetch data for the charts. The one taking the longes is the data request.
        let resps = http.batch([
            ["GET", "http://localhost:19999/api/v1/info", { tags: { fast: "yes" } }],
            ["GET", "http://localhost:19999/api/v1/charts", { tags: { fast: "yes" } }],
            ["GET", "http://localhost:19999/api/v1/data?chart="+charts[rnd(chartmin,chartmax)]+"&before=-"+rnd(beforemin,beforemax)+"&after=-"+rnd(aftermin,aftermax)+"&points="+rnd(pointsmin,pointsmax)+"&format=json&group=average&gtime=0&options=ms%7Cflip%7Cjsonwrap%7Cnonzero&_="+rnd(1,1000000000000), { }],
            ["GET", "http://localhost:19999/api/v1/alarms", { tags: { fast: "yes" } }]
        ]);
        // Combine check() call with failure tracking
        failureRate.add(!check(resps, {
            "status is 200": (r) => r[0].status === 200 && r[1].status === 200
        }));
    });

    sleep(Math.random() * 2 + 1); // Random sleep between 1s and 3s
}
