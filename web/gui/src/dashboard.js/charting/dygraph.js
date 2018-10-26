// dygraph

NETDATA.dygraph = {
    smooth: false
};

NETDATA.dygraphToolboxPanAndZoom = function (state, after, before) {
    if (after < state.netdata_first) {
        after = state.netdata_first;
    }

    if (before > state.netdata_last) {
        before = state.netdata_last;
    }

    state.setMode('zoom');
    NETDATA.globalSelectionSync.stop();
    NETDATA.globalSelectionSync.delay();
    state.tmp.dygraph_user_action = true;
    state.tmp.dygraph_force_zoom = true;
    // state.log('toolboxPanAndZoom');
    state.updateChartPanOrZoom(after, before);
    NETDATA.globalPanAndZoom.setMaster(state, after, before);
};

NETDATA.dygraphSetSelection = function (state, t) {
    if (typeof state.tmp.dygraph_instance !== 'undefined') {
        let r = state.calculateRowForTime(t);
        if (r !== -1) {
            state.tmp.dygraph_instance.setSelection(r);
            return true;
        } else {
            state.tmp.dygraph_instance.clearSelection();
            state.legendShowUndefined();
        }
    }

    return false;
};

NETDATA.dygraphClearSelection = function (state) {
    if (typeof state.tmp.dygraph_instance !== 'undefined') {
        state.tmp.dygraph_instance.clearSelection();
    }
    return true;
};

NETDATA.dygraphSmoothInitialize = function (callback) {
    $.ajax({
        url: NETDATA.dygraph_smooth_js,
        cache: true,
        dataType: "script",
        xhrFields: {withCredentials: true} // required for the cookie
    })
        .done(function () {
            NETDATA.dygraph.smooth = true;
            smoothPlotter.smoothing = 0.3;
        })
        .fail(function () {
            NETDATA.dygraph.smooth = false;
        })
        .always(function () {
            if (typeof callback === "function") {
                return callback();
            }
        });
};

NETDATA.dygraphInitialize = function (callback) {
    if (typeof netdataNoDygraphs === 'undefined' || !netdataNoDygraphs) {
        $.ajax({
            url: NETDATA.dygraph_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('dygraph', NETDATA.dygraph_js);
            })
            .fail(function () {
                NETDATA.chartLibraries.dygraph.enabled = false;
                NETDATA.error(100, NETDATA.dygraph_js);
            })
            .always(function () {
                if (NETDATA.chartLibraries.dygraph.enabled && NETDATA.options.current.smooth_plot) {
                    NETDATA.dygraphSmoothInitialize(callback);
                } else if (typeof callback === "function") {
                    return callback();
                }
            });
    } else {
        NETDATA.chartLibraries.dygraph.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.dygraphChartUpdate = function (state, data) {
    let dygraph = state.tmp.dygraph_instance;

    if (typeof dygraph === 'undefined') {
        return NETDATA.dygraphChartCreate(state, data);
    }

    // when the chart is not visible, and hidden
    // if there is a window resize, dygraph detects
    // its element size as 0x0.
    // this will make it re-appear properly

    if (state.tm.last_unhidden > state.tmp.dygraph_last_rendered) {
        dygraph.resize();
    }

    let options = {
        file: data.result.data,
        colors: state.chartColors(),
        labels: data.result.labels,
        //labelsDivWidth: state.chartWidth() - 70,
        includeZero: state.tmp.dygraph_include_zero,
        visibility: state.dimensions_visibility.selected2BooleanArray(state.data.dimension_names)
    };

    if (state.tmp.dygraph_chart_type === 'stacked') {
        if (options.includeZero && state.dimensions_visibility.countSelected() < options.visibility.length) {
            options.includeZero = 0;
        }
    }

    if (!NETDATA.chartLibraries.dygraph.isSparkline(state)) {
        options.ylabel = state.units_current; // (state.units_desired === 'auto')?"":state.units_current;
    }

    if (state.tmp.dygraph_force_zoom) {
        if (NETDATA.options.debug.dygraph || state.debug) {
            state.log('dygraphChartUpdate() forced zoom update');
        }

        options.dateWindow = (state.requested_padding !== null) ? [state.view_after, state.view_before] : null;
        //options.isZoomedIgnoreProgrammaticZoom = true;
        state.tmp.dygraph_force_zoom = false;
    } else if (state.current.name !== 'auto') {
        if (NETDATA.options.debug.dygraph || state.debug) {
            state.log('dygraphChartUpdate() loose update');
        }
    } else {
        if (NETDATA.options.debug.dygraph || state.debug) {
            state.log('dygraphChartUpdate() strict update');
        }

        options.dateWindow = (state.requested_padding !== null) ? [state.view_after, state.view_before] : null;
        //options.isZoomedIgnoreProgrammaticZoom = true;
    }

    options.valueRange = state.tmp.dygraph_options.valueRange;

    let oldMax = null, oldMin = null;
    if (state.tmp.__commonMin !== null) {
        state.data.min = state.tmp.dygraph_instance.axes_[0].extremeRange[0];
        oldMin = options.valueRange[0] = NETDATA.commonMin.get(state);
    }
    if (state.tmp.__commonMax !== null) {
        state.data.max = state.tmp.dygraph_instance.axes_[0].extremeRange[1];
        oldMax = options.valueRange[1] = NETDATA.commonMax.get(state);
    }

    if (state.tmp.dygraph_smooth_eligible) {
        if ((NETDATA.options.current.smooth_plot && state.tmp.dygraph_options.plotter !== smoothPlotter)
            || (NETDATA.options.current.smooth_plot === false && state.tmp.dygraph_options.plotter === smoothPlotter)) {
            NETDATA.dygraphChartCreate(state, data);
            return;
        }
    }

    if (netdataSnapshotData !== null && NETDATA.globalPanAndZoom.isActive() && NETDATA.globalPanAndZoom.isMaster(state) === false) {
        // pan and zoom on snapshots
        options.dateWindow = [NETDATA.globalPanAndZoom.force_after_ms, NETDATA.globalPanAndZoom.force_before_ms];
        //options.isZoomedIgnoreProgrammaticZoom = true;
    }

    if (NETDATA.chartLibraries.dygraph.isLogScale(state)) {
        if (Array.isArray(options.valueRange) && options.valueRange[0] <= 0) {
            options.valueRange[0] = null;
        }
    }

    dygraph.updateOptions(options);

    let redraw = false;
    if (oldMin !== null && oldMin > state.tmp.dygraph_instance.axes_[0].extremeRange[0]) {
        state.data.min = state.tmp.dygraph_instance.axes_[0].extremeRange[0];
        options.valueRange[0] = NETDATA.commonMin.get(state);
        redraw = true;
    }
    if (oldMax !== null && oldMax < state.tmp.dygraph_instance.axes_[0].extremeRange[1]) {
        state.data.max = state.tmp.dygraph_instance.axes_[0].extremeRange[1];
        options.valueRange[1] = NETDATA.commonMax.get(state);
        redraw = true;
    }

    if (redraw) {
        // state.log('forcing redraw to adapt to common- min/max');
        dygraph.updateOptions(options);
    }

    state.tmp.dygraph_last_rendered = Date.now();
    return true;
};

NETDATA.dygraphChartCreate = function (state, data) {
    if (NETDATA.options.debug.dygraph || state.debug) {
        state.log('dygraphChartCreate()');
    }

    state.tmp.dygraph_chart_type = NETDATA.dataAttribute(state.element, 'dygraph-type', state.chart.chart_type);
    if (state.tmp.dygraph_chart_type === 'stacked' && data.dimensions === 1) {
        state.tmp.dygraph_chart_type = 'area';
    }
    if (state.tmp.dygraph_chart_type === 'stacked' && NETDATA.chartLibraries.dygraph.isLogScale(state)) {
        state.tmp.dygraph_chart_type = 'area';
    }

    let highlightCircleSize = NETDATA.chartLibraries.dygraph.isSparkline(state) ? 3 : 4;

    let smooth = NETDATA.dygraph.smooth
        ? (NETDATA.dataAttributeBoolean(state.element, 'dygraph-smooth', (state.tmp.dygraph_chart_type === 'line' && NETDATA.chartLibraries.dygraph.isSparkline(state) === false)))
        : false;

    state.tmp.dygraph_include_zero = NETDATA.dataAttribute(state.element, 'dygraph-includezero', (state.tmp.dygraph_chart_type === 'stacked'));
    let drawAxis = NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawaxis', true);

    state.tmp.dygraph_options = {
        colors: NETDATA.dataAttribute(state.element, 'dygraph-colors', state.chartColors()),

        // leave a few pixels empty on the right of the chart
        rightGap: NETDATA.dataAttribute(state.element, 'dygraph-rightgap', 5),
        showRangeSelector: NETDATA.dataAttributeBoolean(state.element, 'dygraph-showrangeselector', false),
        showRoller: NETDATA.dataAttributeBoolean(state.element, 'dygraph-showroller', false),
        title: NETDATA.dataAttribute(state.element, 'dygraph-title', state.title),
        titleHeight: NETDATA.dataAttribute(state.element, 'dygraph-titleheight', 19),
        legend: NETDATA.dataAttribute(state.element, 'dygraph-legend', 'always'), // we need this to get selection events
        labels: data.result.labels,
        labelsDiv: NETDATA.dataAttribute(state.element, 'dygraph-labelsdiv', state.element_legend_childs.hidden),
        //labelsDivStyles:        NETDATA.dataAttribute(state.element, 'dygraph-labelsdivstyles', { 'fontSize':'1px' }),
        //labelsDivWidth:         NETDATA.dataAttribute(state.element, 'dygraph-labelsdivwidth', state.chartWidth() - 70),
        labelsSeparateLines: NETDATA.dataAttributeBoolean(state.element, 'dygraph-labelsseparatelines', true),
        labelsShowZeroValues: NETDATA.chartLibraries.dygraph.isLogScale(state) ? false : NETDATA.dataAttributeBoolean(state.element, 'dygraph-labelsshowzerovalues', true),
        labelsKMB: false,
        labelsKMG2: false,
        showLabelsOnHighlight: NETDATA.dataAttributeBoolean(state.element, 'dygraph-showlabelsonhighlight', true),
        hideOverlayOnMouseOut: NETDATA.dataAttributeBoolean(state.element, 'dygraph-hideoverlayonmouseout', true),
        includeZero: state.tmp.dygraph_include_zero,
        xRangePad: NETDATA.dataAttribute(state.element, 'dygraph-xrangepad', 0),
        yRangePad: NETDATA.dataAttribute(state.element, 'dygraph-yrangepad', 1),
        valueRange: NETDATA.dataAttribute(state.element, 'dygraph-valuerange', [null, null]),
        ylabel: state.units_current, // (state.units_desired === 'auto')?"":state.units_current,
        yLabelWidth: NETDATA.dataAttribute(state.element, 'dygraph-ylabelwidth', 12),

        // the function to plot the chart
        plotter: null,

        // The width of the lines connecting data points.
        // This can be used to increase the contrast or some graphs.
        strokeWidth: NETDATA.dataAttribute(state.element, 'dygraph-strokewidth', ((state.tmp.dygraph_chart_type === 'stacked') ? 0.1 : ((smooth === true) ? 1.5 : 0.7))),
        strokePattern: NETDATA.dataAttribute(state.element, 'dygraph-strokepattern', undefined),

        // The size of the dot to draw on each point in pixels (see drawPoints).
        // A dot is always drawn when a point is "isolated",
        // i.e. there is a missing point on either side of it.
        // This also controls the size of those dots.
        drawPoints: NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawpoints', false),

        // Draw points at the edges of gaps in the data.
        // This improves visibility of small data segments or other data irregularities.
        drawGapEdgePoints: NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawgapedgepoints', true),
        connectSeparatedPoints: NETDATA.chartLibraries.dygraph.isLogScale(state) ? false : NETDATA.dataAttributeBoolean(state.element, 'dygraph-connectseparatedpoints', false),
        pointSize: NETDATA.dataAttribute(state.element, 'dygraph-pointsize', 1),

        // enabling this makes the chart with little square lines
        stepPlot: NETDATA.dataAttributeBoolean(state.element, 'dygraph-stepplot', false),

        // Draw a border around graph lines to make crossing lines more easily
        // distinguishable. Useful for graphs with many lines.
        strokeBorderColor: NETDATA.dataAttribute(state.element, 'dygraph-strokebordercolor', NETDATA.themes.current.background),
        strokeBorderWidth: NETDATA.dataAttribute(state.element, 'dygraph-strokeborderwidth', (state.tmp.dygraph_chart_type === 'stacked') ? 0.0 : 0.0),
        fillGraph: NETDATA.dataAttribute(state.element, 'dygraph-fillgraph', (state.tmp.dygraph_chart_type === 'area' || state.tmp.dygraph_chart_type === 'stacked')),
        fillAlpha: NETDATA.dataAttribute(state.element, 'dygraph-fillalpha',
            ((state.tmp.dygraph_chart_type === 'stacked')
                ? NETDATA.options.current.color_fill_opacity_stacked
                : NETDATA.options.current.color_fill_opacity_area)
        ),
        stackedGraph: NETDATA.dataAttribute(state.element, 'dygraph-stackedgraph', (state.tmp.dygraph_chart_type === 'stacked')),
        stackedGraphNaNFill: NETDATA.dataAttribute(state.element, 'dygraph-stackedgraphnanfill', 'none'),
        drawAxis: drawAxis,
        axisLabelFontSize: NETDATA.dataAttribute(state.element, 'dygraph-axislabelfontsize', 10),
        axisLineColor: NETDATA.dataAttribute(state.element, 'dygraph-axislinecolor', NETDATA.themes.current.axis),
        axisLineWidth: NETDATA.dataAttribute(state.element, 'dygraph-axislinewidth', 1.0),
        drawGrid: NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawgrid', true),
        gridLinePattern: NETDATA.dataAttribute(state.element, 'dygraph-gridlinepattern', null),
        gridLineWidth: NETDATA.dataAttribute(state.element, 'dygraph-gridlinewidth', 1.0),
        gridLineColor: NETDATA.dataAttribute(state.element, 'dygraph-gridlinecolor', NETDATA.themes.current.grid),
        maxNumberWidth: NETDATA.dataAttribute(state.element, 'dygraph-maxnumberwidth', 8),
        sigFigs: NETDATA.dataAttribute(state.element, 'dygraph-sigfigs', null),
        digitsAfterDecimal: NETDATA.dataAttribute(state.element, 'dygraph-digitsafterdecimal', 2),
        valueFormatter: NETDATA.dataAttribute(state.element, 'dygraph-valueformatter', undefined),
        highlightCircleSize: NETDATA.dataAttribute(state.element, 'dygraph-highlightcirclesize', highlightCircleSize),
        highlightSeriesOpts: NETDATA.dataAttribute(state.element, 'dygraph-highlightseriesopts', null), // TOO SLOW: { strokeWidth: 1.5 },
        highlightSeriesBackgroundAlpha: NETDATA.dataAttribute(state.element, 'dygraph-highlightseriesbackgroundalpha', null), // TOO SLOW: (state.tmp.dygraph_chart_type === 'stacked')?0.7:0.5,
        pointClickCallback: NETDATA.dataAttribute(state.element, 'dygraph-pointclickcallback', undefined),
        visibility: state.dimensions_visibility.selected2BooleanArray(state.data.dimension_names),
        logscale: NETDATA.chartLibraries.dygraph.isLogScale(state) ? 'y' : undefined,

        axes: {
            x: {
                pixelsPerLabel: NETDATA.dataAttribute(state.element, 'dygraph-xpixelsperlabel', 50),
                ticker: Dygraph.dateTicker,
                axisLabelWidth: NETDATA.dataAttribute(state.element, 'dygraph-xaxislabelwidth', 60),
                drawAxis: NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawxaxis', drawAxis),
                axisLabelFormatter: function (d, gran) {
                    void(gran);
                    return NETDATA.dateTime.xAxisTimeString(d);
                }
            },
            y: {
                logscale: NETDATA.chartLibraries.dygraph.isLogScale(state) ? true : undefined,
                pixelsPerLabel: NETDATA.dataAttribute(state.element, 'dygraph-ypixelsperlabel', 15),
                axisLabelWidth: NETDATA.dataAttribute(state.element, 'dygraph-yaxislabelwidth', 50),
                drawAxis: NETDATA.dataAttributeBoolean(state.element, 'dygraph-drawyaxis', drawAxis),
                axisLabelFormatter: function (y) {

                    // unfortunately, we have to call this every single time
                    state.legendFormatValueDecimalsFromMinMax(
                        this.axes_[0].extremeRange[0],
                        this.axes_[0].extremeRange[1]
                    );

                    let old_units = this.user_attrs_.ylabel;
                    let v = state.legendFormatValue(y);
                    let new_units = state.units_current;

                    if (state.units_desired === 'auto' && typeof old_units !== 'undefined' && new_units !== old_units && !NETDATA.chartLibraries.dygraph.isSparkline(state)) {
                        // console.log(this);
                        // state.log('units discrepancy: old = ' + old_units + ', new = ' + new_units);
                        let len = this.plugins_.length;
                        while (len--) {
                            // console.log(this.plugins_[len]);
                            if (typeof this.plugins_[len].plugin.ylabel_div_ !== 'undefined'
                                && this.plugins_[len].plugin.ylabel_div_ !== null
                                && typeof this.plugins_[len].plugin.ylabel_div_.children !== 'undefined'
                                && this.plugins_[len].plugin.ylabel_div_.children !== null
                                && typeof this.plugins_[len].plugin.ylabel_div_.children[0].children !== 'undefined'
                                && this.plugins_[len].plugin.ylabel_div_.children[0].children !== null
                            ) {
                                this.plugins_[len].plugin.ylabel_div_.children[0].children[0].innerHTML = new_units;
                                this.user_attrs_.ylabel = new_units;
                                break;
                            }
                        }

                        if (len < 0) {
                            state.log('units discrepancy, but cannot find dygraphs div to change: old = ' + old_units + ', new = ' + new_units);
                        }
                    }

                    return v;
                }
            }
        },
        legendFormatter: function (data) {
            if (state.tmp.dygraph_mouse_down) {
                return;
            }

            let elements = state.element_legend_childs;

            // if the hidden div is not there
            // we are not managing the legend
            if (elements.hidden === null) {
                return;
            }

            if (typeof data.x !== 'undefined') {
                state.legendSetDate(data.x);
                let i = data.series.length;
                while (i--) {
                    let series = data.series[i];
                    if (series.isVisible) {
                        state.legendSetLabelValue(series.label, series.y);
                    } else {
                        state.legendSetLabelValue(series.label, null);
                    }
                }
            }

            return '';
        },
        drawCallback: function (dygraph, is_initial) {

            // the user has panned the chart and this is called to re-draw the chart
            // 1. refresh this chart by adding data to it
            // 2. notify all the other charts about the update they need

            // to prevent an infinite loop (feedback), we use
            //     state.tmp.dygraph_user_action
            // - when true, this is initiated by a user
            // - when false, this is feedback

            if (state.current.name !== 'auto' && state.tmp.dygraph_user_action) {
                state.tmp.dygraph_user_action = false;

                let x_range = dygraph.xAxisRange();
                let after = Math.round(x_range[0]);
                let before = Math.round(x_range[1]);

                if (NETDATA.options.debug.dygraph) {
                    state.log('dygraphDrawCallback(dygraph, ' + is_initial + '): mode ' + state.current.name + ' ' + (after / 1000).toString() + ' - ' + (before / 1000).toString());
                    //console.log(state);
                }

                if (before <= state.netdata_last && after >= state.netdata_first) {
                    // update only when we are within the data limits
                    state.updateChartPanOrZoom(after, before);
                }
            }
        },
        zoomCallback: function (minDate, maxDate, yRanges) {

            // the user has selected a range on the chart
            // 1. refresh this chart by adding data to it
            // 2. notify all the other charts about the update they need

            void(yRanges);

            if (NETDATA.options.debug.dygraph) {
                state.log('dygraphZoomCallback(): ' + state.current.name);
            }

            NETDATA.globalSelectionSync.stop();
            NETDATA.globalSelectionSync.delay();
            state.setMode('zoom');

            // refresh it to the greatest possible zoom level
            state.tmp.dygraph_user_action = true;
            state.tmp.dygraph_force_zoom = true;
            state.updateChartPanOrZoom(minDate, maxDate);
        },
        highlightCallback: function (event, x, points, row, seriesName) {
            void(seriesName);

            state.pauseChart();

            // there is a bug in dygraph when the chart is zoomed enough
            // the time it thinks is selected is wrong
            // here we calculate the time t based on the row number selected
            // which is ok
            // let t = state.data_after + row * state.data_update_every;
            // console.log('row = ' + row + ', x = ' + x + ', t = ' + t + ' ' + ((t === x)?'SAME':(Math.abs(x-t)<=state.data_update_every)?'SIMILAR':'DIFFERENT') + ', rows in db: ' + state.data_points + ' visible(x) = ' + state.timeIsVisible(x) + ' visible(t) = ' + state.timeIsVisible(t) + ' r(x) = ' + state.calculateRowForTime(x) + ' r(t) = ' + state.calculateRowForTime(t) + ' range: ' + state.data_after + ' - ' + state.data_before + ' real: ' + state.data.after + ' - ' + state.data.before + ' every: ' + state.data_update_every);

            if (state.tmp.dygraph_mouse_down !== true) {
                NETDATA.globalSelectionSync.sync(state, x);
            }

            // fix legend zIndex using the internal structures of dygraph legend module
            // this works, but it is a hack!
            // state.tmp.dygraph_instance.plugins_[0].plugin.legend_div_.style.zIndex = 10000;
        },
        unhighlightCallback: function (event) {
            void(event);

            if (state.tmp.dygraph_mouse_down) {
                return;
            }

            if (NETDATA.options.debug.dygraph || state.debug) {
                state.log('dygraphUnhighlightCallback()');
            }

            state.unpauseChart();
            NETDATA.globalSelectionSync.stop();
        },
        underlayCallback: function (canvas, area, g) {

            // the chart is about to be drawn
            // this function renders global highlighted time-frame

            if (NETDATA.globalChartUnderlay.isActive()) {
                let after = NETDATA.globalChartUnderlay.after;
                let before = NETDATA.globalChartUnderlay.before;

                if (after < state.view_after) {
                    after = state.view_after;
                }

                if (before > state.view_before) {
                    before = state.view_before;
                }

                if (after < before) {
                    let bottom_left = g.toDomCoords(after, -20);
                    let top_right = g.toDomCoords(before, +20);

                    let left = bottom_left[0];
                    let right = top_right[0];

                    canvas.fillStyle = NETDATA.themes.current.highlight;
                    canvas.fillRect(left, area.y, right - left, area.h);
                }
            }
        },
        interactionModel: {
            mousedown: function (event, dygraph, context) {
                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.mousedown()');
                }

                state.tmp.dygraph_user_action = true;

                if (NETDATA.options.debug.dygraph) {
                    state.log('dygraphMouseDown()');
                }

                // Right-click should not initiate anything.
                if (event.button && event.button === 2) {
                    return;
                }

                NETDATA.globalSelectionSync.stop();
                NETDATA.globalSelectionSync.delay();

                state.tmp.dygraph_mouse_down = true;
                context.initializeMouseDown(event, dygraph, context);

                //console.log(event);
                if (event.button && event.button === 1) {
                    if (event.shiftKey) {
                        //console.log('middle mouse button dragging (PAN)');

                        state.setMode('pan');
                        // NETDATA.globalSelectionSync.delay();
                        state.tmp.dygraph_highlight_after = null;
                        Dygraph.startPan(event, dygraph, context);
                    } else if (event.altKey || event.ctrlKey || event.metaKey) {
                        //console.log('middle mouse button highlight');

                        if (!(event.offsetX && event.offsetY)) {
                            event.offsetX = event.layerX - event.target.offsetLeft;
                            event.offsetY = event.layerY - event.target.offsetTop;
                        }
                        state.tmp.dygraph_highlight_after = dygraph.toDataXCoord(event.offsetX);
                        Dygraph.startZoom(event, dygraph, context);
                    } else {
                        //console.log('middle mouse button selection for zoom (ZOOM)');

                        state.setMode('zoom');
                        // NETDATA.globalSelectionSync.delay();
                        state.tmp.dygraph_highlight_after = null;
                        Dygraph.startZoom(event, dygraph, context);
                    }
                } else {
                    if (event.shiftKey) {
                        //console.log('left mouse button selection for zoom (ZOOM)');

                        state.setMode('zoom');
                        // NETDATA.globalSelectionSync.delay();
                        state.tmp.dygraph_highlight_after = null;
                        Dygraph.startZoom(event, dygraph, context);
                    } else if (event.altKey || event.ctrlKey || event.metaKey) {
                        //console.log('left mouse button highlight');

                        if (!(event.offsetX && event.offsetY)) {
                            event.offsetX = event.layerX - event.target.offsetLeft;
                            event.offsetY = event.layerY - event.target.offsetTop;
                        }
                        state.tmp.dygraph_highlight_after = dygraph.toDataXCoord(event.offsetX);
                        Dygraph.startZoom(event, dygraph, context);
                    } else {
                        //console.log('left mouse button dragging (PAN)');

                        state.setMode('pan');
                        // NETDATA.globalSelectionSync.delay();
                        state.tmp.dygraph_highlight_after = null;
                        Dygraph.startPan(event, dygraph, context);
                    }
                }
            },
            mousemove: function (event, dygraph, context) {
                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.mousemove()');
                }

                if (state.tmp.dygraph_highlight_after !== null) {
                    //console.log('highlight selection...');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    state.tmp.dygraph_user_action = true;
                    Dygraph.moveZoom(event, dygraph, context);
                    event.preventDefault();
                } else if (context.isPanning) {
                    //console.log('panning...');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    state.tmp.dygraph_user_action = true;
                    //NETDATA.globalSelectionSync.stop();
                    //NETDATA.globalSelectionSync.delay();
                    state.setMode('pan');
                    context.is2DPan = false;
                    Dygraph.movePan(event, dygraph, context);
                } else if (context.isZooming) {
                    //console.log('zooming...');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    state.tmp.dygraph_user_action = true;
                    //NETDATA.globalSelectionSync.stop();
                    //NETDATA.globalSelectionSync.delay();
                    state.setMode('zoom');
                    Dygraph.moveZoom(event, dygraph, context);
                }
            },
            mouseup: function (event, dygraph, context) {
                state.tmp.dygraph_mouse_down = false;

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.mouseup()');
                }

                if (state.tmp.dygraph_highlight_after !== null) {
                    //console.log('done highlight selection');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    if (!(event.offsetX && event.offsetY)) {
                        event.offsetX = event.layerX - event.target.offsetLeft;
                        event.offsetY = event.layerY - event.target.offsetTop;
                    }

                    NETDATA.globalChartUnderlay.set(state
                        , state.tmp.dygraph_highlight_after
                        , dygraph.toDataXCoord(event.offsetX)
                        , state.view_after
                        , state.view_before
                    );

                    state.tmp.dygraph_highlight_after = null;

                    context.isZooming = false;
                    dygraph.clearZoomRect_();
                    dygraph.drawGraph_(false);

                    // refresh all the charts immediately
                    NETDATA.options.auto_refresher_stop_until = 0;
                } else if (context.isPanning) {
                    //console.log('done panning');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    state.tmp.dygraph_user_action = true;
                    Dygraph.endPan(event, dygraph, context);

                    // refresh all the charts immediately
                    NETDATA.options.auto_refresher_stop_until = 0;
                } else if (context.isZooming) {
                    //console.log('done zomming');

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    state.tmp.dygraph_user_action = true;
                    Dygraph.endZoom(event, dygraph, context);

                    // refresh all the charts immediately
                    NETDATA.options.auto_refresher_stop_until = 0;
                }
            },
            click: function (event, dygraph, context) {
                void(dygraph);
                void(context);

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.click()');
                }

                event.preventDefault();
            },
            dblclick: function (event, dygraph, context) {
                void(event);
                void(dygraph);
                void(context);

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.dblclick()');
                }
                NETDATA.resetAllCharts(state);
            },
            wheel: function (event, dygraph, context) {
                void(context);

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.wheel()');
                }

                // Take the offset of a mouse event on the dygraph canvas and
                // convert it to a pair of percentages from the bottom left.
                // (Not top left, bottom is where the lower value is.)
                function offsetToPercentage(g, offsetX, offsetY) {
                    // This is calculating the pixel offset of the leftmost date.
                    let xOffset = g.toDomCoords(g.xAxisRange()[0], null)[0];
                    let yar0 = g.yAxisRange(0);

                    // This is calculating the pixel of the highest value. (Top pixel)
                    let yOffset = g.toDomCoords(null, yar0[1])[1];

                    // x y w and h are relative to the corner of the drawing area,
                    // so that the upper corner of the drawing area is (0, 0).
                    let x = offsetX - xOffset;
                    let y = offsetY - yOffset;

                    // This is computing the rightmost pixel, effectively defining the
                    // width.
                    let w = g.toDomCoords(g.xAxisRange()[1], null)[0] - xOffset;

                    // This is computing the lowest pixel, effectively defining the height.
                    let h = g.toDomCoords(null, yar0[0])[1] - yOffset;

                    // Percentage from the left.
                    let xPct = w === 0 ? 0 : (x / w);
                    // Percentage from the top.
                    let yPct = h === 0 ? 0 : (y / h);

                    // The (1-) part below changes it from "% distance down from the top"
                    // to "% distance up from the bottom".
                    return [xPct, (1 - yPct)];
                }

                // Adjusts [x, y] toward each other by zoomInPercentage%
                // Split it so the left/bottom axis gets xBias/yBias of that change and
                // tight/top gets (1-xBias)/(1-yBias) of that change.
                //
                // If a bias is missing it splits it down the middle.
                function zoomRange(g, zoomInPercentage, xBias, yBias) {
                    xBias = xBias || 0.5;
                    yBias = yBias || 0.5;

                    function adjustAxis(axis, zoomInPercentage, bias) {
                        let delta = axis[1] - axis[0];
                        let increment = delta * zoomInPercentage;
                        let foo = [increment * bias, increment * (1 - bias)];

                        return [axis[0] + foo[0], axis[1] - foo[1]];
                    }

                    let yAxes = g.yAxisRanges();
                    let newYAxes = [];
                    for (let i = 0; i < yAxes.length; i++) {
                        newYAxes[i] = adjustAxis(yAxes[i], zoomInPercentage, yBias);
                    }

                    return adjustAxis(g.xAxisRange(), zoomInPercentage, xBias);
                }

                if (event.altKey || event.shiftKey) {
                    state.tmp.dygraph_user_action = true;

                    NETDATA.globalSelectionSync.stop();
                    NETDATA.globalSelectionSync.delay();

                    // http://dygraphs.com/gallery/interaction-api.js
                    let normal_def;
                    if (typeof event.wheelDelta === 'number' && !isNaN(event.wheelDelta))
                    // chrome
                    {
                        normal_def = event.wheelDelta / 40;
                    } else
                    // firefox
                    {
                        normal_def = event.deltaY * -1.2;
                    }

                    let normal = (event.detail) ? event.detail * -1 : normal_def;
                    let percentage = normal / 50;

                    if (!(event.offsetX && event.offsetY)) {
                        event.offsetX = event.layerX - event.target.offsetLeft;
                        event.offsetY = event.layerY - event.target.offsetTop;
                    }

                    let percentages = offsetToPercentage(dygraph, event.offsetX, event.offsetY);
                    let xPct = percentages[0];
                    let yPct = percentages[1];

                    let new_x_range = zoomRange(dygraph, percentage, xPct, yPct);
                    let after = new_x_range[0];
                    let before = new_x_range[1];

                    let first = state.netdata_first + state.data_update_every;
                    let last = state.netdata_last + state.data_update_every;

                    if (before > last) {
                        after -= (before - last);
                        before = last;
                    }
                    if (after < first) {
                        after = first;
                    }

                    state.setMode('zoom');
                    state.updateChartPanOrZoom(after, before, function () {
                        dygraph.updateOptions({dateWindow: [after, before]});
                    });

                    event.preventDefault();
                }
            },
            touchstart: function (event, dygraph, context) {
                state.tmp.dygraph_mouse_down = true;

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.touchstart()');
                }

                state.tmp.dygraph_user_action = true;
                state.setMode('zoom');
                state.pauseChart();

                NETDATA.globalSelectionSync.stop();
                NETDATA.globalSelectionSync.delay();

                Dygraph.defaultInteractionModel.touchstart(event, dygraph, context);

                // we overwrite the touch directions at the end, to overwrite
                // the internal default of dygraph
                context.touchDirections = {x: true, y: false};

                state.dygraph_last_touch_start = Date.now();
                state.dygraph_last_touch_move = 0;

                if (typeof event.touches[0].pageX === 'number') {
                    state.dygraph_last_touch_page_x = event.touches[0].pageX;
                } else {
                    state.dygraph_last_touch_page_x = 0;
                }
            },
            touchmove: function (event, dygraph, context) {
                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.touchmove()');
                }

                NETDATA.globalSelectionSync.stop();
                NETDATA.globalSelectionSync.delay();

                state.tmp.dygraph_user_action = true;
                Dygraph.defaultInteractionModel.touchmove(event, dygraph, context);

                state.dygraph_last_touch_move = Date.now();
            },
            touchend: function (event, dygraph, context) {
                state.tmp.dygraph_mouse_down = false;

                if (NETDATA.options.debug.dygraph || state.debug) {
                    state.log('interactionModel.touchend()');
                }

                NETDATA.globalSelectionSync.stop();
                NETDATA.globalSelectionSync.delay();

                state.tmp.dygraph_user_action = true;
                Dygraph.defaultInteractionModel.touchend(event, dygraph, context);

                // if it didn't move, it is a selection
                if (state.dygraph_last_touch_move === 0 && state.dygraph_last_touch_page_x !== 0) {
                    NETDATA.globalSelectionSync.dont_sync_before = 0;
                    NETDATA.globalSelectionSync.setMaster(state);

                    // internal api of dygraph
                    let pct = (state.dygraph_last_touch_page_x - (dygraph.plotter_.area.x + state.element.getBoundingClientRect().left)) / dygraph.plotter_.area.w;
                    console.log('pct: ' + pct.toString());

                    let t = Math.round(state.view_after + (state.view_before - state.view_after) * pct);
                    if (NETDATA.dygraphSetSelection(state, t)) {
                        NETDATA.globalSelectionSync.sync(state, t);
                    }
                }

                // if it was double tap within double click time, reset the charts
                let now = Date.now();
                if (typeof state.dygraph_last_touch_end !== 'undefined') {
                    if (state.dygraph_last_touch_move === 0) {
                        let dt = now - state.dygraph_last_touch_end;
                        if (dt <= NETDATA.options.current.double_click_speed) {
                            NETDATA.resetAllCharts(state);
                        }
                    }
                }

                // remember the timestamp of the last touch end
                state.dygraph_last_touch_end = now;

                // refresh all the charts immediately
                NETDATA.options.auto_refresher_stop_until = 0;
            }
        }
    };

    if (NETDATA.chartLibraries.dygraph.isLogScale(state)) {
        if (Array.isArray(state.tmp.dygraph_options.valueRange) && state.tmp.dygraph_options.valueRange[0] <= 0) {
            state.tmp.dygraph_options.valueRange[0] = null;
        }
    }

    if (NETDATA.chartLibraries.dygraph.isSparkline(state)) {
        state.tmp.dygraph_options.drawGrid = false;
        state.tmp.dygraph_options.drawAxis = false;
        state.tmp.dygraph_options.title = undefined;
        state.tmp.dygraph_options.ylabel = undefined;
        state.tmp.dygraph_options.yLabelWidth = 0;
        //state.tmp.dygraph_options.labelsDivWidth = 120;
        //state.tmp.dygraph_options.labelsDivStyles.width = '120px';
        state.tmp.dygraph_options.labelsSeparateLines = true;
        state.tmp.dygraph_options.rightGap = 0;
        state.tmp.dygraph_options.yRangePad = 1;
        state.tmp.dygraph_options.axes.x.drawAxis = false;
        state.tmp.dygraph_options.axes.y.drawAxis = false;
    }

    if (smooth) {
        state.tmp.dygraph_smooth_eligible = true;

        if (NETDATA.options.current.smooth_plot) {
            state.tmp.dygraph_options.plotter = smoothPlotter;
        }
    }
    else {
        state.tmp.dygraph_smooth_eligible = false;
    }

    if (netdataSnapshotData !== null && NETDATA.globalPanAndZoom.isActive() && NETDATA.globalPanAndZoom.isMaster(state) === false) {
        // pan and zoom on snapshots
        state.tmp.dygraph_options.dateWindow = [NETDATA.globalPanAndZoom.force_after_ms, NETDATA.globalPanAndZoom.force_before_ms];
        //state.tmp.dygraph_options.isZoomedIgnoreProgrammaticZoom = true;
    }

    state.tmp.dygraph_instance = new Dygraph(state.element_chart,
        data.result.data, state.tmp.dygraph_options);

    state.tmp.dygraph_force_zoom = false;
    state.tmp.dygraph_user_action = false;
    state.tmp.dygraph_last_rendered = Date.now();
    state.tmp.dygraph_highlight_after = null;

    if (state.tmp.dygraph_options.valueRange[0] === null && state.tmp.dygraph_options.valueRange[1] === null) {
        if (typeof state.tmp.dygraph_instance.axes_[0].extremeRange !== 'undefined') {
            state.tmp.__commonMin = NETDATA.dataAttribute(state.element, 'common-min', null);
            state.tmp.__commonMax = NETDATA.dataAttribute(state.element, 'common-max', null);
        } else {
            state.log('incompatible version of Dygraph detected');
            state.tmp.__commonMin = null;
            state.tmp.__commonMax = null;
        }
    } else {
        // if the user gave a valueRange, respect it
        state.tmp.__commonMin = null;
        state.tmp.__commonMax = null;
    }

    return true;
};
