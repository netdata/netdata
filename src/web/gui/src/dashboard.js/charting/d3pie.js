
// d3pie

NETDATA.d3pieInitialize = function (callback) {
    if (typeof netdataNoD3pie === 'undefined' || !netdataNoD3pie) {

        // d3pie requires D3
        if (!NETDATA.chartLibraries.d3.initialized) {
            if (NETDATA.chartLibraries.d3.enabled) {
                NETDATA.d3Initialize(function () {
                    NETDATA.d3pieInitialize(callback);
                });
            } else {
                NETDATA.chartLibraries.d3pie.enabled = false;
                if (typeof callback === "function") {
                    return callback();
                }
            }
        } else {
            $.ajax({
                url: NETDATA.d3pie_js,
                cache: true,
                dataType: "script",
                xhrFields: {withCredentials: true} // required for the cookie
            })
                .done(function () {
                    NETDATA.registerChartLibrary('d3pie', NETDATA.d3pie_js);
                })
                .fail(function () {
                    NETDATA.chartLibraries.d3pie.enabled = false;
                    NETDATA.error(100, NETDATA.d3pie_js);
                })
                .always(function () {
                    if (typeof callback === "function") {
                        return callback();
                    }
                });
        }
    } else {
        NETDATA.chartLibraries.d3pie.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.d3pieSetContent = function (state, data, index) {
    state.legendFormatValueDecimalsFromMinMax(
        data.min,
        data.max
    );

    let content = [];
    let colors = state.chartColors();
    let len = data.result.labels.length;
    for (let i = 1; i < len; i++) {
        let label = data.result.labels[i];
        let value = data.result.data[index][label];
        let color = colors[i - 1];

        if (value !== null && value > 0) {
            content.push({
                label: label,
                value: value,
                color: color
            });
        }
    }

    if (content.length === 0) {
        content.push({
            label: 'no data',
            value: 100,
            color: '#666666'
        });
    }

    state.tmp.d3pie_last_slot = index;
    return content;
};

NETDATA.d3pieDateRange = function (state, data, index) {
    let dt = Math.round((data.before - data.after + 1) / data.points);
    let dt_str = NETDATA.seconds4human(dt);

    let before = data.result.data[index].time;
    let after = before - (dt * 1000);

    let d1 = NETDATA.dateTime.localeDateString(after);
    let t1 = NETDATA.dateTime.localeTimeString(after);
    let d2 = NETDATA.dateTime.localeDateString(before);
    let t2 = NETDATA.dateTime.localeTimeString(before);

    if (d1 === d2) {
        return d1 + ' ' + t1 + ' to ' + t2 + ', ' + dt_str;
    }

    return d1 + ' ' + t1 + ' to ' + d2 + ' ' + t2 + ', ' + dt_str;
};

NETDATA.d3pieSetSelection = function (state, t) {
    if (state.timeIsVisible(t) !== true) {
        return NETDATA.d3pieClearSelection(state, true);
    }

    let slot = state.calculateRowForTime(t);
    slot = state.data.result.data.length - slot - 1;

    if (slot < 0 || slot >= state.data.result.length) {
        return NETDATA.d3pieClearSelection(state, true);
    }

    if (state.tmp.d3pie_last_slot === slot) {
        // we already show this slot, don't do anything
        return true;
    }

    if (state.tmp.d3pie_timer === undefined) {
        state.tmp.d3pie_timer = NETDATA.timeout.set(function () {
            state.tmp.d3pie_timer = undefined;
            NETDATA.d3pieChange(state, NETDATA.d3pieSetContent(state, state.data, slot), NETDATA.d3pieDateRange(state, state.data, slot));
        }, 0);
    }

    return true;
};

NETDATA.d3pieClearSelection = function (state, force) {
    if (typeof state.tmp.d3pie_timer !== 'undefined') {
        NETDATA.timeout.clear(state.tmp.d3pie_timer);
        state.tmp.d3pie_timer = undefined;
    }

    if (state.isAutoRefreshable() && state.data !== null && force !== true) {
        NETDATA.d3pieChartUpdate(state, state.data);
    } else {
        if (state.tmp.d3pie_last_slot !== -1) {
            state.tmp.d3pie_last_slot = -1;
            NETDATA.d3pieChange(state, [{label: 'no data', value: 1, color: '#666666'}], 'no data available');
        }
    }

    return true;
};

NETDATA.d3pieChange = function (state, content, footer) {
    if (state.d3pie_forced_subtitle === null) {
        //state.d3pie_instance.updateProp("header.subtitle.text", state.units_current);
        state.d3pie_instance.options.header.subtitle.text = state.units_current;
    }

    if (state.d3pie_forced_footer === null) {
        //state.d3pie_instance.updateProp("footer.text", footer);
        state.d3pie_instance.options.footer.text = footer;
    }

    //state.d3pie_instance.updateProp("data.content", content);
    state.d3pie_instance.options.data.content = content;
    state.d3pie_instance.destroy();
    state.d3pie_instance.recreate();
    return true;
};

NETDATA.d3pieChartUpdate = function (state, data) {
    return NETDATA.d3pieChange(state, NETDATA.d3pieSetContent(state, data, 0), NETDATA.d3pieDateRange(state, data, 0));
};

NETDATA.d3pieChartCreate = function (state, data) {

    state.element_chart.id = 'd3pie-' + state.uuid;
    // console.log('id = ' + state.element_chart.id);

    let content = NETDATA.d3pieSetContent(state, data, 0);

    state.d3pie_forced_title = NETDATA.dataAttribute(state.element, 'd3pie-title', null);
    state.d3pie_forced_subtitle = NETDATA.dataAttribute(state.element, 'd3pie-subtitle', null);
    state.d3pie_forced_footer = NETDATA.dataAttribute(state.element, 'd3pie-footer', null);

    state.d3pie_options = {
        header: {
            title: {
                text: (state.d3pie_forced_title !== null) ? state.d3pie_forced_title : state.title,
                color: NETDATA.dataAttribute(state.element, 'd3pie-title-color', NETDATA.themes.current.d3pie.title),
                fontSize: NETDATA.dataAttribute(state.element, 'd3pie-title-fontsize', 12),
                fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-title-fontweight', "bold"),
                font: NETDATA.dataAttribute(state.element, 'd3pie-title-font', "arial")
            },
            subtitle: {
                text: (state.d3pie_forced_subtitle !== null) ? state.d3pie_forced_subtitle : state.units_current,
                color: NETDATA.dataAttribute(state.element, 'd3pie-subtitle-color', NETDATA.themes.current.d3pie.subtitle),
                fontSize: NETDATA.dataAttribute(state.element, 'd3pie-subtitle-fontsize', 10),
                fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-subtitle-fontweight', "normal"),
                font: NETDATA.dataAttribute(state.element, 'd3pie-subtitle-font', "arial")
            },
            titleSubtitlePadding: 1
        },
        footer: {
            text: (state.d3pie_forced_footer !== null) ? state.d3pie_forced_footer : NETDATA.d3pieDateRange(state, data, 0),
            color: NETDATA.dataAttribute(state.element, 'd3pie-footer-color', NETDATA.themes.current.d3pie.footer),
            fontSize: NETDATA.dataAttribute(state.element, 'd3pie-footer-fontsize', 9),
            fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-footer-fontweight', "bold"),
            font: NETDATA.dataAttribute(state.element, 'd3pie-footer-font', "arial"),
            location: NETDATA.dataAttribute(state.element, 'd3pie-footer-location', "bottom-center") // bottom-left, bottom-center, bottom-right
        },
        size: {
            canvasHeight: state.chartHeight(),
            canvasWidth: state.chartWidth(),
            pieInnerRadius: NETDATA.dataAttribute(state.element, 'd3pie-pieinnerradius', "45%"),
            pieOuterRadius: NETDATA.dataAttribute(state.element, 'd3pie-pieouterradius', "80%")
        },
        data: {
            // none, random, value-asc, value-desc, label-asc, label-desc
            sortOrder: NETDATA.dataAttribute(state.element, 'd3pie-sortorder', "value-desc"),
            smallSegmentGrouping: {
                enabled: NETDATA.dataAttributeBoolean(state.element, "d3pie-smallsegmentgrouping-enabled", false),
                value: NETDATA.dataAttribute(state.element, 'd3pie-smallsegmentgrouping-value', 1),
                // percentage, value
                valueType: NETDATA.dataAttribute(state.element, 'd3pie-smallsegmentgrouping-valuetype', "percentage"),
                label: NETDATA.dataAttribute(state.element, 'd3pie-smallsegmentgrouping-label', "other"),
                color: NETDATA.dataAttribute(state.element, 'd3pie-smallsegmentgrouping-color', NETDATA.themes.current.d3pie.other)
            },

            // REQUIRED! This is where you enter your pie data; it needs to be an array of objects
            // of this form: { label: "label", value: 1.5, color: "#000000" } - color is optional
            content: content
        },
        labels: {
            outer: {
                // label, value, percentage, label-value1, label-value2, label-percentage1, label-percentage2
                format: NETDATA.dataAttribute(state.element, 'd3pie-labels-outer-format', "label-value1"),
                hideWhenLessThanPercentage: NETDATA.dataAttribute(state.element, 'd3pie-labels-outer-hidewhenlessthanpercentage', null),
                pieDistance: NETDATA.dataAttribute(state.element, 'd3pie-labels-outer-piedistance', 15)
            },
            inner: {
                // label, value, percentage, label-value1, label-value2, label-percentage1, label-percentage2
                format: NETDATA.dataAttribute(state.element, 'd3pie-labels-inner-format', "percentage"),
                hideWhenLessThanPercentage: NETDATA.dataAttribute(state.element, 'd3pie-labels-inner-hidewhenlessthanpercentage', 2)
            },
            mainLabel: {
                color: NETDATA.dataAttribute(state.element, 'd3pie-labels-mainLabel-color', NETDATA.themes.current.d3pie.mainlabel), // or 'segment' for dynamic color
                font: NETDATA.dataAttribute(state.element, 'd3pie-labels-mainLabel-font', "arial"),
                fontSize: NETDATA.dataAttribute(state.element, 'd3pie-labels-mainLabel-fontsize', 10),
                fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-labels-mainLabel-fontweight', "normal")
            },
            percentage: {
                color: NETDATA.dataAttribute(state.element, 'd3pie-labels-percentage-color', NETDATA.themes.current.d3pie.percentage),
                font: NETDATA.dataAttribute(state.element, 'd3pie-labels-percentage-font', "arial"),
                fontSize: NETDATA.dataAttribute(state.element, 'd3pie-labels-percentage-fontsize', 10),
                fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-labels-percentage-fontweight', "bold"),
                decimalPlaces: 0
            },
            value: {
                color: NETDATA.dataAttribute(state.element, 'd3pie-labels-value-color', NETDATA.themes.current.d3pie.value),
                font: NETDATA.dataAttribute(state.element, 'd3pie-labels-value-font', "arial"),
                fontSize: NETDATA.dataAttribute(state.element, 'd3pie-labels-value-fontsize', 10),
                fontWeight: NETDATA.dataAttribute(state.element, 'd3pie-labels-value-fontweight', "bold")
            },
            lines: {
                enabled: NETDATA.dataAttributeBoolean(state.element, 'd3pie-labels-lines-enabled', true),
                style: NETDATA.dataAttribute(state.element, 'd3pie-labels-lines-style', "curved"),
                color: NETDATA.dataAttribute(state.element, 'd3pie-labels-lines-color', "segment") // "segment" or a hex color
            },
            truncation: {
                enabled: NETDATA.dataAttributeBoolean(state.element, 'd3pie-labels-truncation-enabled', false),
                truncateLength: NETDATA.dataAttribute(state.element, 'd3pie-labels-truncation-truncatelength', 30)
            },
            formatter: function (context) {
                // console.log(context);
                if (context.part === 'value') {
                    return state.legendFormatValue(context.value);
                }
                if (context.part === 'percentage') {
                    return context.label + '%';
                }

                return context.label;
            }
        },
        effects: {
            load: {
                effect: "none", // none / default
                speed: 0 // commented in the d3pie code to speed it up
            },
            pullOutSegmentOnClick: {
                effect: "bounce", // none / linear / bounce / elastic / back
                speed: 400,
                size: 5
            },
            highlightSegmentOnMouseover: true,
            highlightLuminosity: -0.2
        },
        tooltips: {
            enabled: false,
            type: "placeholder", // caption|placeholder
            string: "",
            placeholderParser: null, // function
            styles: {
                fadeInSpeed: 250,
                backgroundColor: NETDATA.themes.current.d3pie.tooltip_bg,
                backgroundOpacity: 0.5,
                color: NETDATA.themes.current.d3pie.tooltip_fg,
                borderRadius: 2,
                font: "arial",
                fontSize: 12,
                padding: 4
            }
        },
        misc: {
            colors: {
                background: 'transparent', // transparent or color #
                // segments: state.chartColors(),
                segmentStroke: NETDATA.dataAttribute(state.element, 'd3pie-misc-colors-segmentstroke', NETDATA.themes.current.d3pie.segment_stroke)
            },
            gradient: {
                enabled: NETDATA.dataAttributeBoolean(state.element, 'd3pie-misc-gradient-enabled', false),
                percentage: NETDATA.dataAttribute(state.element, 'd3pie-misc-colors-percentage', 95),
                color: NETDATA.dataAttribute(state.element, 'd3pie-misc-gradient-color', NETDATA.themes.current.d3pie.gradient_color)
            },
            canvasPadding: {
                top: 5,
                right: 5,
                bottom: 5,
                left: 5
            },
            pieCenterOffset: {
                x: 0,
                y: 0
            },
            cssPrefix: NETDATA.dataAttribute(state.element, 'd3pie-cssprefix', null)
        },
        callbacks: {
            onload: null,
            onMouseoverSegment: null,
            onMouseoutSegment: null,
            onClickSegment: null
        }
    };

    state.d3pie_instance = new d3pie(state.element_chart, state.d3pie_options);
    return true;
};
