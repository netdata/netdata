
// DEPRECATED: will be removed!

// raphael

NETDATA.raphaelInitialize = function(callback) {
    if (typeof netdataStopRaphael === 'undefined' || !netdataStopRaphael) {
        $.ajax({
            url: NETDATA.raphael_js,
            cache: true,
            dataType: "script",
            xhrFields: { withCredentials: true } // required for the cookie
        })
        .done(function() {
            NETDATA.registerChartLibrary('raphael', NETDATA.raphael_js);
        })
        .fail(function() {
            NETDATA.chartLibraries.raphael.enabled = false;
            NETDATA.error(100, NETDATA.raphael_js);
        })
        .always(function() {
            if (typeof callback === "function")
                return callback();
        });
    } else {
        NETDATA.chartLibraries.raphael.enabled = false;
        if (typeof callback === "function")
            return callback();
    }
};

NETDATA.raphaelChartUpdate = function(state, data) {
    $(state.element_chart).raphael(data.result, {
        width: state.chartWidth(),
        height: state.chartHeight()
    });

    return false;
};

NETDATA.raphaelChartCreate = function(state, data) {
    $(state.element_chart).raphael(data.result, {
        width: state.chartWidth(),
        height: state.chartHeight()
    });

    return false;
};
