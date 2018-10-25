
// DEPRECATED: will be removed!

// morris

NETDATA.morrisInitialize = function(callback) {
    if (typeof netdataNoMorris === 'undefined' || !netdataNoMorris) {

        // morris requires raphael
        if (!NETDATA.chartLibraries.raphael.initialized) {
            if (NETDATA.chartLibraries.raphael.enabled) {
                NETDATA.raphaelInitialize(function() {
                    NETDATA.morrisInitialize(callback);
                });
            } else {
                NETDATA.chartLibraries.morris.enabled = false;
                if (typeof callback === "function")
                    return callback();
            }
        } else {
            NETDATA._loadCSS(NETDATA.morris_css);

            $.ajax({
                url: NETDATA.morris_js,
                cache: true,
                dataType: "script",
                xhrFields: { withCredentials: true } // required for the cookie
            })
            .done(function() {
                NETDATA.registerChartLibrary('morris', NETDATA.morris_js);
            })
            .fail(function() {
                NETDATA.chartLibraries.morris.enabled = false;
                NETDATA.error(100, NETDATA.morris_js);
            })
            .always(function() {
                if (typeof callback === "function")
                    return callback();
            });
        }
    } else {
        NETDATA.chartLibraries.morris.enabled = false;
        if (typeof callback === "function")
            return callback();
    }
};

NETDATA.morrisChartUpdate = function(state, data) {
    state.morris_instance.setData(data.result.data);
    return true;
};

NETDATA.morrisChartCreate = function(state, data) {

    state.morris_options = {
            element: state.element_chart.id,
            data: data.result.data,
            xkey: 'time',
            ykeys: data.dimension_names,
            labels: data.dimension_names,
            lineWidth: 2,
            pointSize: 3,
            smooth: true,
            hideHover: 'auto',
            parseTime: true,
            continuousLine: false,
            behaveLikeLine: false
    };

    if (state.chart.chart_type === 'line')
        state.morris_instance = new Morris.Line(state.morris_options);

    else if (state.chart.chart_type === 'area') {
        state.morris_options.behaveLikeLine = true;
        state.morris_instance = new Morris.Area(state.morris_options);
    }
    else // stacked
        state.morris_instance = new Morris.Area(state.morris_options);

    return true;
};
