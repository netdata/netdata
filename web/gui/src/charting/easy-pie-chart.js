// ----------------------------------------------------------------------------------------------------------------

NETDATA.easypiechartPercentFromValueMinMax = function (state, value, min, max) {
    if (typeof value !== 'number') {
        value = 0;
    }
    if (typeof min !== 'number') {
        min = 0;
    }
    if (typeof max !== 'number') {
        max = 0;
    }

    if (min > max) {
        let t = min;
        min = max;
        max = t;
    }

    if (min > value) {
        min = value;
    }
    if (max < value) {
        max = value;
    }

    state.legendFormatValueDecimalsFromMinMax(min, max);

    if (state.tmp.easyPieChartMin === null && min > 0) {
        min = 0;
    }
    if (state.tmp.easyPieChartMax === null && max < 0) {
        max = 0;
    }

    let pcent;

    if (min < 0 && max > 0) {
        // it is both positive and negative
        // zero at the top center of the chart
        max = (-min > max) ? -min : max;
        pcent = Math.round(value * 100 / max);
    } else if (value >= 0 && min >= 0 && max >= 0) {
        // clockwise
        pcent = Math.round((value - min) * 100 / (max - min));
        if (pcent === 0) {
            pcent = 0.1;
        }
    } else {
        // counter clockwise
        pcent = Math.round((value - max) * 100 / (max - min));
        if (pcent === 0) {
            pcent = -0.1;
        }
    }

    return pcent;
};

// ----------------------------------------------------------------------------------------------------------------
// easy-pie-chart

NETDATA.easypiechartInitialize = function (callback) {
    if (typeof netdataNoEasyPieChart === 'undefined' || !netdataNoEasyPieChart) {
        $.ajax({
            url: NETDATA.easypiechart_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('easypiechart', NETDATA.easypiechart_js);
            })
            .fail(function () {
                NETDATA.chartLibraries.easypiechart.enabled = false;
                NETDATA.error(100, NETDATA.easypiechart_js);
            })
            .always(function () {
                if (typeof callback === "function") {
                    return callback();
                }
            })
    } else {
        NETDATA.chartLibraries.easypiechart.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.easypiechartClearSelection = function (state, force) {
    if (typeof state.tmp.easyPieChartEvent !== 'undefined' && typeof state.tmp.easyPieChartEvent.timer !== 'undefined') {
        NETDATA.timeout.clear(state.tmp.easyPieChartEvent.timer);
        state.tmp.easyPieChartEvent.timer = undefined;
    }

    if (state.isAutoRefreshable() && state.data !== null && force !== true) {
        NETDATA.easypiechartChartUpdate(state, state.data);
    }
    else {
        state.tmp.easyPieChartLabel.innerText = state.legendFormatValue(null);
        state.tmp.easyPieChart_instance.update(0);
    }
    state.tmp.easyPieChart_instance.enableAnimation();

    return true;
};

NETDATA.easypiechartSetSelection = function (state, t) {
    if (state.timeIsVisible(t) !== true) {
        return NETDATA.easypiechartClearSelection(state, true);
    }

    let slot = state.calculateRowForTime(t);
    if (slot < 0 || slot >= state.data.result.length) {
        return NETDATA.easypiechartClearSelection(state, true);
    }

    if (typeof state.tmp.easyPieChartEvent === 'undefined') {
        state.tmp.easyPieChartEvent = {
            timer: undefined,
            value: 0,
            pcent: 0
        };
    }

    let value = state.data.result[state.data.result.length - 1 - slot];
    let min = (state.tmp.easyPieChartMin === null) ? NETDATA.commonMin.get(state) : state.tmp.easyPieChartMin;
    let max = (state.tmp.easyPieChartMax === null) ? NETDATA.commonMax.get(state) : state.tmp.easyPieChartMax;
    let pcent = NETDATA.easypiechartPercentFromValueMinMax(state, value, min, max);

    state.tmp.easyPieChartEvent.value = value;
    state.tmp.easyPieChartEvent.pcent = pcent;
    state.tmp.easyPieChartLabel.innerText = state.legendFormatValue(value);

    if (state.tmp.easyPieChartEvent.timer === undefined) {
        state.tmp.easyPieChart_instance.disableAnimation();

        state.tmp.easyPieChartEvent.timer = NETDATA.timeout.set(function () {
            state.tmp.easyPieChartEvent.timer = undefined;
            state.tmp.easyPieChart_instance.update(state.tmp.easyPieChartEvent.pcent);
        }, 0);
    }

    return true;
};

NETDATA.easypiechartChartUpdate = function (state, data) {
    let value, min, max, pcent;

    if (NETDATA.globalPanAndZoom.isActive() || state.isAutoRefreshable() === false) {
        value = null;
        pcent = 0;
    }
    else {
        value = data.result[0];
        min = (state.tmp.easyPieChartMin === null) ? NETDATA.commonMin.get(state) : state.tmp.easyPieChartMin;
        max = (state.tmp.easyPieChartMax === null) ? NETDATA.commonMax.get(state) : state.tmp.easyPieChartMax;
        pcent = NETDATA.easypiechartPercentFromValueMinMax(state, value, min, max);
    }

    state.tmp.easyPieChartLabel.innerText = state.legendFormatValue(value);
    state.tmp.easyPieChart_instance.update(pcent);
    return true;
};

NETDATA.easypiechartChartCreate = function (state, data) {
    let chart = $(state.element_chart);

    let value = data.result[0];
    let min = NETDATA.dataAttribute(state.element, 'easypiechart-min-value', null);
    let max = NETDATA.dataAttribute(state.element, 'easypiechart-max-value', null);

    if (min === null) {
        min = NETDATA.commonMin.get(state);
        state.tmp.easyPieChartMin = null;
    }
    else {
        state.tmp.easyPieChartMin = min;
    }

    if (max === null) {
        max = NETDATA.commonMax.get(state);
        state.tmp.easyPieChartMax = null;
    }
    else {
        state.tmp.easyPieChartMax = max;
    }

    let size = state.chartWidth();
    let stroke = Math.floor(size / 22);
    if (stroke < 3) {
        stroke = 2;
    }

    let valuefontsize = Math.floor((size * 2 / 3) / 5);
    let valuetop = Math.round((size - valuefontsize - (size / 40)) / 2);
    state.tmp.easyPieChartLabel = document.createElement('span');
    state.tmp.easyPieChartLabel.className = 'easyPieChartLabel';
    state.tmp.easyPieChartLabel.innerText = state.legendFormatValue(value);
    state.tmp.easyPieChartLabel.style.fontSize = valuefontsize + 'px';
    state.tmp.easyPieChartLabel.style.top = valuetop.toString() + 'px';
    state.element_chart.appendChild(state.tmp.easyPieChartLabel);

    let titlefontsize = Math.round(valuefontsize * 1.6 / 3);
    let titletop = Math.round(valuetop - (titlefontsize * 2) - (size / 40));
    state.tmp.easyPieChartTitle = document.createElement('span');
    state.tmp.easyPieChartTitle.className = 'easyPieChartTitle';
    state.tmp.easyPieChartTitle.innerText = state.title;
    state.tmp.easyPieChartTitle.style.fontSize = titlefontsize + 'px';
    state.tmp.easyPieChartTitle.style.lineHeight = titlefontsize + 'px';
    state.tmp.easyPieChartTitle.style.top = titletop.toString() + 'px';
    state.element_chart.appendChild(state.tmp.easyPieChartTitle);

    let unitfontsize = Math.round(titlefontsize * 0.9);
    let unittop = Math.round(valuetop + (valuefontsize + unitfontsize) + (size / 40));
    state.tmp.easyPieChartUnits = document.createElement('span');
    state.tmp.easyPieChartUnits.className = 'easyPieChartUnits';
    state.tmp.easyPieChartUnits.innerText = state.units_current;
    state.tmp.easyPieChartUnits.style.fontSize = unitfontsize + 'px';
    state.tmp.easyPieChartUnits.style.top = unittop.toString() + 'px';
    state.element_chart.appendChild(state.tmp.easyPieChartUnits);

    let barColor = NETDATA.dataAttribute(state.element, 'easypiechart-barcolor', undefined);
    if (typeof barColor === 'undefined' || barColor === null) {
        barColor = state.chartCustomColors()[0];
    } else {
        // <div ... data-easypiechart-barcolor="(function(percent){return(percent < 50 ? '#5cb85c' : percent < 85 ? '#f0ad4e' : '#cb3935');})" ...></div>
        let tmp = eval(barColor);
        if (typeof tmp === 'function') {
            barColor = tmp;
        }
    }

    let pcent = NETDATA.easypiechartPercentFromValueMinMax(state, value, min, max);
    chart.data('data-percent', pcent);

    chart.easyPieChart({
        barColor: barColor,
        trackColor: NETDATA.dataAttribute(state.element, 'easypiechart-trackcolor', NETDATA.themes.current.easypiechart_track),
        scaleColor: NETDATA.dataAttribute(state.element, 'easypiechart-scalecolor', NETDATA.themes.current.easypiechart_scale),
        scaleLength: NETDATA.dataAttribute(state.element, 'easypiechart-scalelength', 5),
        lineCap: NETDATA.dataAttribute(state.element, 'easypiechart-linecap', 'round'),
        lineWidth: NETDATA.dataAttribute(state.element, 'easypiechart-linewidth', stroke),
        trackWidth: NETDATA.dataAttribute(state.element, 'easypiechart-trackwidth', undefined),
        size: NETDATA.dataAttribute(state.element, 'easypiechart-size', size),
        rotate: NETDATA.dataAttribute(state.element, 'easypiechart-rotate', 0),
        animate: NETDATA.dataAttribute(state.element, 'easypiechart-animate', {duration: 500, enabled: true}),
        easing: NETDATA.dataAttribute(state.element, 'easypiechart-easing', undefined)
    });

    // when we just re-create the chart
    // do not animate the first update
    let animate = true;
    if (typeof state.tmp.easyPieChart_instance !== 'undefined') {
        animate = false;
    }

    state.tmp.easyPieChart_instance = chart.data('easyPieChart');
    if (animate === false) {
        state.tmp.easyPieChart_instance.disableAnimation();
    }
    state.tmp.easyPieChart_instance.update(pcent);
    if (animate === false) {
        state.tmp.easyPieChart_instance.enableAnimation();
    }

    state.legendSetUnitsString = function (units) {
        if (typeof state.tmp.easyPieChartUnits !== 'undefined' && state.tmp.units !== units) {
            state.tmp.easyPieChartUnits.innerText = units;
            state.tmp.units = units;
        }
    };
    state.legendShowUndefined = function () {
        if (typeof state.tmp.easyPieChart_instance !== 'undefined') {
            NETDATA.easypiechartClearSelection(state);
        }
    };

    return true;
};
