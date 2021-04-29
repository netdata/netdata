
// peity

NETDATA.peityInitialize = function (callback) {
    if (typeof netdataNoPeitys === 'undefined' || !netdataNoPeitys) {
        $.ajax({
            url: NETDATA.peity_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('peity', NETDATA.peity_js);
            })
            .fail(function () {
                NETDATA.chartLibraries.peity.enabled = false;
                NETDATA.error(100, NETDATA.peity_js);
            })
            .always(function () {
                if (typeof callback === "function") {
                    return callback();
                }
            });
    } else {
        NETDATA.chartLibraries.peity.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.peityChartUpdate = function (state, data) {
    state.peity_instance.innerHTML = data.result;

    if (state.peity_options.stroke !== state.chartCustomColors()[0]) {
        state.peity_options.stroke = state.chartCustomColors()[0];
        if (state.chart.chart_type === 'line') {
            state.peity_options.fill = NETDATA.themes.current.background;
        } else {
            state.peity_options.fill = NETDATA.colorLuminance(state.chartCustomColors()[0], NETDATA.chartDefaults.fill_luminance);
        }
    }

    $(state.peity_instance).peity('line', state.peity_options);
    return true;
};

NETDATA.peityChartCreate = function (state, data) {
    state.peity_instance = document.createElement('div');
    state.element_chart.appendChild(state.peity_instance);

    state.peity_options = {
        stroke: NETDATA.themes.current.foreground,
        strokeWidth: NETDATA.dataAttribute(state.element, 'peity-strokewidth', 1),
        width: state.chartWidth(),
        height: state.chartHeight(),
        fill: NETDATA.themes.current.foreground
    };

    NETDATA.peityChartUpdate(state, data);
    return true;
};
