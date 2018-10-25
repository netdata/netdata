// gauge.js

NETDATA.gaugeInitialize = function (callback) {
    if (typeof netdataNoGauge === 'undefined' || !netdataNoGauge) {
        $.ajax({
            url: NETDATA.gauge_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('gauge', NETDATA.gauge_js);
            })
            .fail(function () {
                NETDATA.chartLibraries.gauge.enabled = false;
                NETDATA.error(100, NETDATA.gauge_js);
            })
            .always(function () {
                if (typeof callback === "function") {
                    return callback();
                }
            })
    }
    else {
        NETDATA.chartLibraries.gauge.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.gaugeAnimation = function (state, status) {
    let speed = 32;

    if (typeof status === 'boolean' && status === false) {
        speed = 1000000000;
    } else if (typeof status === 'number') {
        speed = status;
    }

    // console.log('gauge speed ' + speed);
    state.tmp.gauge_instance.animationSpeed = speed;
    state.tmp.___gaugeOld__.speed = speed;
};

NETDATA.gaugeSet = function (state, value, min, max) {
    if (typeof value !== 'number') {
        value = 0;
    }
    if (typeof min !== 'number') {
        min = 0;
    }
    if (typeof max !== 'number') {
        max = 0;
    }
    if (value > max) {
        max = value;
    }
    if (value < min) {
        min = value;
    }
    if (min > max) {
        let t = min;
        min = max;
        max = t;
    }
    else if (min === max) {
        max = min + 1;
    }

    state.legendFormatValueDecimalsFromMinMax(min, max);

    // gauge.js has an issue if the needle
    // is smaller than min or larger than max
    // when we set the new values
    // the needle will go crazy

    // to prevent it, we always feed it
    // with a percentage, so that the needle
    // is always between min and max
    let pcent = (value - min) * 100 / (max - min);

    // bug fix for gauge.js 1.3.1
    // if the value is the absolute min or max, the chart is broken
    if (pcent < 0.001) {
        pcent = 0.001;
    }
    if (pcent > 99.999) {
        pcent = 99.999;
    }

    state.tmp.gauge_instance.set(pcent);
    // console.log('gauge set ' + pcent + ', value ' + value + ', min ' + min + ', max ' + max);

    state.tmp.___gaugeOld__.value = value;
    state.tmp.___gaugeOld__.min = min;
    state.tmp.___gaugeOld__.max = max;
};

NETDATA.gaugeSetLabels = function (state, value, min, max) {
    if (state.tmp.___gaugeOld__.valueLabel !== value) {
        state.tmp.___gaugeOld__.valueLabel = value;
        state.tmp.gaugeChartLabel.innerText = state.legendFormatValue(value);
    }
    if (state.tmp.___gaugeOld__.minLabel !== min) {
        state.tmp.___gaugeOld__.minLabel = min;
        state.tmp.gaugeChartMin.innerText = state.legendFormatValue(min);
    }
    if (state.tmp.___gaugeOld__.maxLabel !== max) {
        state.tmp.___gaugeOld__.maxLabel = max;
        state.tmp.gaugeChartMax.innerText = state.legendFormatValue(max);
    }
};

NETDATA.gaugeClearSelection = function (state, force) {
    if (typeof state.tmp.gaugeEvent !== 'undefined' && typeof state.tmp.gaugeEvent.timer !== 'undefined') {
        NETDATA.timeout.clear(state.tmp.gaugeEvent.timer);
        state.tmp.gaugeEvent.timer = undefined;
    }

    if (state.isAutoRefreshable() && state.data !== null && force !== true) {
        NETDATA.gaugeChartUpdate(state, state.data);
    } else {
        NETDATA.gaugeAnimation(state, false);
        NETDATA.gaugeSetLabels(state, null, null, null);
        NETDATA.gaugeSet(state, null, null, null);
    }

    NETDATA.gaugeAnimation(state, true);
    return true;
};

NETDATA.gaugeSetSelection = function (state, t) {
    if (state.timeIsVisible(t) !== true) {
        return NETDATA.gaugeClearSelection(state, true);
    }

    let slot = state.calculateRowForTime(t);
    if (slot < 0 || slot >= state.data.result.length) {
        return NETDATA.gaugeClearSelection(state, true);
    }

    if (typeof state.tmp.gaugeEvent === 'undefined') {
        state.tmp.gaugeEvent = {
            timer: undefined,
            value: 0,
            min: 0,
            max: 0
        };
    }

    let value = state.data.result[state.data.result.length - 1 - slot];
    let min = (state.tmp.gaugeMin === null) ? NETDATA.commonMin.get(state) : state.tmp.gaugeMin;
    let max = (state.tmp.gaugeMax === null) ? NETDATA.commonMax.get(state) : state.tmp.gaugeMax;

    // make sure it is zero based
    // but only if it has not been set by the user
    if (state.tmp.gaugeMin === null && min > 0) {
        min = 0;
    }
    if (state.tmp.gaugeMax === null && max < 0) {
        max = 0;
    }

    state.tmp.gaugeEvent.value = value;
    state.tmp.gaugeEvent.min = min;
    state.tmp.gaugeEvent.max = max;
    NETDATA.gaugeSetLabels(state, value, min, max);

    if (state.tmp.gaugeEvent.timer === undefined) {
        NETDATA.gaugeAnimation(state, false);

        state.tmp.gaugeEvent.timer = NETDATA.timeout.set(function () {
            state.tmp.gaugeEvent.timer = undefined;
            NETDATA.gaugeSet(state, state.tmp.gaugeEvent.value, state.tmp.gaugeEvent.min, state.tmp.gaugeEvent.max);
        }, 0);
    }

    return true;
};

NETDATA.gaugeChartUpdate = function (state, data) {
    let value, min, max;

    if (NETDATA.globalPanAndZoom.isActive() || state.isAutoRefreshable() === false) {
        NETDATA.gaugeSetLabels(state, null, null, null);
        state.tmp.gauge_instance.set(0);
    } else {
        value = data.result[0];
        min = (state.tmp.gaugeMin === null) ? NETDATA.commonMin.get(state) : state.tmp.gaugeMin;
        max = (state.tmp.gaugeMax === null) ? NETDATA.commonMax.get(state) : state.tmp.gaugeMax;
        if (value < min) {
            min = value;
        }
        if (value > max) {
            max = value;
        }

        // make sure it is zero based
        // but only if it has not been set by the user
        if (state.tmp.gaugeMin === null && min > 0) {
            min = 0;
        }
        if (state.tmp.gaugeMax === null && max < 0) {
            max = 0;
        }

        NETDATA.gaugeSet(state, value, min, max);
        NETDATA.gaugeSetLabels(state, value, min, max);
    }

    return true;
};

NETDATA.gaugeChartCreate = function (state, data) {
    // let chart = $(state.element_chart);

    let value = data.result[0];
    let min = NETDATA.dataAttribute(state.element, 'gauge-min-value', null);
    let max = NETDATA.dataAttribute(state.element, 'gauge-max-value', null);
    // let adjust = NETDATA.dataAttribute(state.element, 'gauge-adjust', null);
    let pointerColor = NETDATA.dataAttribute(state.element, 'gauge-pointer-color', NETDATA.themes.current.gauge_pointer);
    let strokeColor = NETDATA.dataAttribute(state.element, 'gauge-stroke-color', NETDATA.themes.current.gauge_stroke);
    let startColor = NETDATA.dataAttribute(state.element, 'gauge-start-color', state.chartCustomColors()[0]);
    let stopColor = NETDATA.dataAttribute(state.element, 'gauge-stop-color', void 0);
    let generateGradient = NETDATA.dataAttribute(state.element, 'gauge-generate-gradient', false);

    if (min === null) {
        min = NETDATA.commonMin.get(state);
        state.tmp.gaugeMin = null;
    } else {
        state.tmp.gaugeMin = min;
    }

    if (max === null) {
        max = NETDATA.commonMax.get(state);
        state.tmp.gaugeMax = null;
    } else {
        state.tmp.gaugeMax = max;
    }

    // make sure it is zero based
    // but only if it has not been set by the user
    if (state.tmp.gaugeMin === null && min > 0) {
        min = 0;
    }
    if (state.tmp.gaugeMax === null && max < 0) {
        max = 0;
    }

    let width = state.chartWidth(), height = state.chartHeight(); //, ratio = 1.5;
    // console.log('gauge width: ' + width.toString() + ', height: ' + height.toString());
    //switch(adjust) {
    //  case 'width': width = height * ratio; break;
    //  case 'height':
    //  default: height = width / ratio; break;
    //}
    //state.element.style.width = width.toString() + 'px';
    //state.element.style.height = height.toString() + 'px';

    let lum_d = 0.05;

    let options = {
        lines: 12,                  // The number of lines to draw
        angle: 0.14,                // The span of the gauge arc
        lineWidth: 0.57,            // The line thickness
        radiusScale: 1.0,           // Relative radius
        pointer: {
            length: 0.85,           // 0.9 The radius of the inner circle
            strokeWidth: 0.045,     // The rotation offset
            color: pointerColor     // Fill color
        },
        limitMax: true,             // If false, the max value of the gauge will be updated if value surpass max
        limitMin: true,             // If true, the min value of the gauge will be fixed unless you set it manually
        colorStart: startColor,     // Colors
        colorStop: stopColor,       // just experiment with them
        strokeColor: strokeColor,   // to see which ones work best for you
        generateGradient: (generateGradient === true), // gmosx: 
        gradientType: 0,
        highDpiSupport: true        // High resolution support
    };

    if (generateGradient.constructor === Array) {
        // example options:
        // data-gauge-generate-gradient="[0, 50, 100]"
        // data-gauge-gradient-percent-color-0="#FFFFFF"
        // data-gauge-gradient-percent-color-50="#999900"
        // data-gauge-gradient-percent-color-100="#000000"

        options.percentColors = [];
        let len = generateGradient.length;
        while (len--) {
            let pcent = generateGradient[len];
            let color = NETDATA.dataAttribute(state.element, 'gauge-gradient-percent-color-' + pcent.toString(), false);
            if (color !== false) {
                let a = [];
                a[0] = pcent / 100;
                a[1] = color;
                options.percentColors.unshift(a);
            }
        }
        if (options.percentColors.length === 0) {
            delete options.percentColors;
        }
    } else if (generateGradient === false && NETDATA.themes.current.gauge_gradient) {
        //noinspection PointlessArithmeticExpressionJS
        options.percentColors = [
            [0.0, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 0))],
            [0.1, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 1))],
            [0.2, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 2))],
            [0.3, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 3))],
            [0.4, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 4))],
            [0.5, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 5))],
            [0.6, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 6))],
            [0.7, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 7))],
            [0.8, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 8))],
            [0.9, NETDATA.colorLuminance(startColor, (lum_d * 10) - (lum_d * 9))],
            [1.0, NETDATA.colorLuminance(startColor, 0.0)]];
    }

    state.tmp.gauge_canvas = document.createElement('canvas');
    state.tmp.gauge_canvas.id = 'gauge-' + state.uuid + '-canvas';
    state.tmp.gauge_canvas.className = 'gaugeChart';
    state.tmp.gauge_canvas.width = width;
    state.tmp.gauge_canvas.height = height;
    state.element_chart.appendChild(state.tmp.gauge_canvas);

    let valuefontsize = Math.floor(height / 5);
    let valuetop = Math.round((height - valuefontsize) / 3.2);
    state.tmp.gaugeChartLabel = document.createElement('span');
    state.tmp.gaugeChartLabel.className = 'gaugeChartLabel';
    state.tmp.gaugeChartLabel.style.fontSize = valuefontsize + 'px';
    state.tmp.gaugeChartLabel.style.top = valuetop.toString() + 'px';
    state.element_chart.appendChild(state.tmp.gaugeChartLabel);

    let titlefontsize = Math.round(valuefontsize / 2.1);
    let titletop = 0;
    state.tmp.gaugeChartTitle = document.createElement('span');
    state.tmp.gaugeChartTitle.className = 'gaugeChartTitle';
    state.tmp.gaugeChartTitle.innerText = state.title;
    state.tmp.gaugeChartTitle.style.fontSize = titlefontsize + 'px';
    state.tmp.gaugeChartTitle.style.lineHeight = titlefontsize + 'px';
    state.tmp.gaugeChartTitle.style.top = titletop.toString() + 'px';
    state.element_chart.appendChild(state.tmp.gaugeChartTitle);

    let unitfontsize = Math.round(titlefontsize * 0.9);
    state.tmp.gaugeChartUnits = document.createElement('span');
    state.tmp.gaugeChartUnits.className = 'gaugeChartUnits';
    state.tmp.gaugeChartUnits.innerText = state.units_current;
    state.tmp.gaugeChartUnits.style.fontSize = unitfontsize + 'px';
    state.element_chart.appendChild(state.tmp.gaugeChartUnits);

    state.tmp.gaugeChartMin = document.createElement('span');
    state.tmp.gaugeChartMin.className = 'gaugeChartMin';
    state.tmp.gaugeChartMin.style.fontSize = Math.round(valuefontsize * 0.75).toString() + 'px';
    state.element_chart.appendChild(state.tmp.gaugeChartMin);

    state.tmp.gaugeChartMax = document.createElement('span');
    state.tmp.gaugeChartMax.className = 'gaugeChartMax';
    state.tmp.gaugeChartMax.style.fontSize = Math.round(valuefontsize * 0.75).toString() + 'px';
    state.element_chart.appendChild(state.tmp.gaugeChartMax);

    // when we just re-create the chart
    // do not animate the first update
    let animate = true;
    if (typeof state.tmp.gauge_instance !== 'undefined') {
        animate = false;
    }

    state.tmp.gauge_instance = new Gauge(state.tmp.gauge_canvas).setOptions(options); // create sexy gauge!

    state.tmp.___gaugeOld__ = {
        value: value,
        min: min,
        max: max,
        valueLabel: null,
        minLabel: null,
        maxLabel: null
    };

    // we will always feed a percentage
    state.tmp.gauge_instance.minValue = 0;
    state.tmp.gauge_instance.maxValue = 100;

    NETDATA.gaugeAnimation(state, animate);
    NETDATA.gaugeSet(state, value, min, max);
    NETDATA.gaugeSetLabels(state, value, min, max);
    NETDATA.gaugeAnimation(state, true);

    state.legendSetUnitsString = function (units) {
        if (typeof state.tmp.gaugeChartUnits !== 'undefined' && state.tmp.units !== units) {
            state.tmp.gaugeChartUnits.innerText = units;
            state.tmp.___gaugeOld__.valueLabel = null;
            state.tmp.___gaugeOld__.minLabel = null;
            state.tmp.___gaugeOld__.maxLabel = null;
            state.tmp.units = units;
        }
    };
    state.legendShowUndefined = function () {
        if (typeof state.tmp.gauge_instance !== 'undefined') {
            NETDATA.gaugeClearSelection(state);
        }
    };

    return true;
};
