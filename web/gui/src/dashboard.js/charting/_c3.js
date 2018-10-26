
// DEPRECATED: will be removed!

// c3

NETDATA.c3Initialize = function(callback) {
    if (typeof netdataNoC3 === 'undefined' || !netdataNoC3) {

        // C3 requires D3
        if (!NETDATA.chartLibraries.d3.initialized) {
            if (NETDATA.chartLibraries.d3.enabled) {
                NETDATA.d3Initialize(function() {
                    NETDATA.c3Initialize(callback);
                });
            } else {
                NETDATA.chartLibraries.c3.enabled = false;
                if (typeof callback === "function")
                    return callback();
            }
        } else {
            NETDATA._loadCSS(NETDATA.c3_css);

            $.ajax({
                url: NETDATA.c3_js,
                cache: true,
                dataType: "script",
                xhrFields: { withCredentials: true } // required for the cookie
            })
            .done(function() {
                NETDATA.registerChartLibrary('c3', NETDATA.c3_js);
            })
            .fail(function() {
                NETDATA.chartLibraries.c3.enabled = false;
                NETDATA.error(100, NETDATA.c3_js);
            })
            .always(function() {
                if (typeof callback === "function")
                    return callback();
            });
        }
    } else {
        NETDATA.chartLibraries.c3.enabled = false;
        if (typeof callback === "function")
            return callback();
    }
};

NETDATA.c3ChartUpdate = function(state, data) {
    state.c3_instance.destroy();
    return NETDATA.c3ChartCreate(state, data);

    //state.c3_instance.load({
    //  rows: data.result,
    //  unload: true
    //});

    //return true;
};

NETDATA.c3ChartCreate = function(state, data) {

    state.element_chart.id = 'c3-' + state.uuid;
    // console.log('id = ' + state.element_chart.id);

    state.c3_instance = c3.generate({
        bindto: '#' + state.element_chart.id,
        size: {
            width: state.chartWidth(),
            height: state.chartHeight()
        },
        color: {
            pattern: state.chartColors()
        },
        data: {
            x: 'time',
            rows: data.result,
            type: (state.chart.chart_type === 'line')?'spline':'area-spline'
        },
        axis: {
            x: {
                type: 'timeseries',
                tick: {
                    format: function(x) {
                        return NETDATA.dateTime.xAxisTimeString(x);
                    }
                }
            }
        },
        grid: {
            x: {
                show: true
            },
            y: {
                show: true
            }
        },
        point: {
            show: false
        },
        line: {
            connectNull: false
        },
        transition: {
            duration: 0
        },
        interaction: {
            enabled: true
        }
    });

    // console.log(state.c3_instance);

    return true;
};
