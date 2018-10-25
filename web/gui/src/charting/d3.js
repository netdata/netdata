
// ----------------------------------------------------------------------------------------------------------------
// D3

NETDATA.d3Initialize = function(callback) {
    if (typeof netdataStopD3 === 'undefined' || !netdataStopD3) {
        $.ajax({
            url: NETDATA.d3_js,
            cache: true,
            dataType: "script",
            xhrFields: { withCredentials: true } // required for the cookie
        })
        .done(function() {
            NETDATA.registerChartLibrary('d3', NETDATA.d3_js);
        })
        .fail(function() {
            NETDATA.chartLibraries.d3.enabled = false;
            NETDATA.error(100, NETDATA.d3_js);
        })
        .always(function() {
            if (typeof callback === "function")
                return callback();
        });
    } else {
        NETDATA.chartLibraries.d3.enabled = false;
        if (typeof callback === "function")
            return callback();
    }
};

NETDATA.d3ChartUpdate = function(state, data) {
    void(state);
    void(data);

    return false;
};

NETDATA.d3ChartCreate = function(state, data) {
    void(state);
    void(data);

    return false;
};
