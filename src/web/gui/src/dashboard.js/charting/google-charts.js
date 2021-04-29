// google charts

// Codacy declarations
/* global google */

NETDATA.googleInitialize = function (callback) {
    if (typeof netdataNoGoogleCharts === 'undefined' || !netdataNoGoogleCharts) {
        $.ajax({
            url: NETDATA.google_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('google', NETDATA.google_js);
                google.load('visualization', '1.1', {
                    'packages': ['corechart', 'controls'],
                    'callback': callback
                });
            })
            .fail(function () {
                NETDATA.chartLibraries.google.enabled = false;
                NETDATA.error(100, NETDATA.google_js);
                if (typeof callback === "function") {
                    return callback();
                }
            });
    } else {
        NETDATA.chartLibraries.google.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.googleChartUpdate = function (state, data) {
    let datatable = new google.visualization.DataTable(data.result);
    state.google_instance.draw(datatable, state.google_options);
    return true;
};

NETDATA.googleChartCreate = function (state, data) {
    let datatable = new google.visualization.DataTable(data.result);

    state.google_options = {
        colors: state.chartColors(),

        // do not set width, height - the chart resizes itself
        //width: state.chartWidth(),
        //height: state.chartHeight(),
        lineWidth: 1,
        title: state.title,
        fontSize: 11,
        hAxis: {
            //  title: "Time of Day",
            //  format:'HH:mm:ss',
            viewWindowMode: 'maximized',
            slantedText: false,
            format: 'HH:mm:ss',
            textStyle: {
                fontSize: 9
            },
            gridlines: {
                color: '#EEE'
            }
        },
        vAxis: {
            title: state.units_current,
            viewWindowMode: 'pretty',
            minValue: -0.1,
            maxValue: 0.1,
            direction: 1,
            textStyle: {
                fontSize: 9
            },
            gridlines: {
                color: '#EEE'
            }
        },
        chartArea: {
            width: '65%',
            height: '80%'
        },
        focusTarget: 'category',
        annotation: {
            '1': {
                style: 'line'
            }
        },
        pointsVisible: 0,
        titlePosition: 'out',
        titleTextStyle: {
            fontSize: 11
        },
        tooltip: {
            isHtml: false,
            ignoreBounds: true,
            textStyle: {
                fontSize: 9
            }
        },
        curveType: 'function',
        areaOpacity: 0.3,
        isStacked: false
    };

    switch (state.chart.chart_type) {
        case "area":
            state.google_options.vAxis.viewWindowMode = 'maximized';
            state.google_options.areaOpacity = NETDATA.options.current.color_fill_opacity_area;
            state.google_instance = new google.visualization.AreaChart(state.element_chart);
            break;

        case "stacked":
            state.google_options.isStacked = true;
            state.google_options.areaOpacity = NETDATA.options.current.color_fill_opacity_stacked;
            state.google_options.vAxis.viewWindowMode = 'maximized';
            state.google_options.vAxis.minValue = null;
            state.google_options.vAxis.maxValue = null;
            state.google_instance = new google.visualization.AreaChart(state.element_chart);
            break;

        default:
        case "line":
            state.google_options.lineWidth = 2;
            state.google_instance = new google.visualization.LineChart(state.element_chart);
            break;
    }

    state.google_instance.draw(datatable, state.google_options);
    return true;
};
