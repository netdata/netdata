// ----------------------------------------------------------------------------------------------------------------
// sparkline

NETDATA.sparklineInitialize = function (callback) {
    if (typeof netdataNoSparklines === 'undefined' || !netdataNoSparklines) {
        $.ajax({
            url: NETDATA.sparkline_js,
            cache: true,
            dataType: "script",
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function () {
                NETDATA.registerChartLibrary('sparkline', NETDATA.sparkline_js);
            })
            .fail(function () {
                NETDATA.chartLibraries.sparkline.enabled = false;
                NETDATA.error(100, NETDATA.sparkline_js);
            })
            .always(function () {
                if (typeof callback === "function") {
                    return callback();
                }
            });
    } else {
        NETDATA.chartLibraries.sparkline.enabled = false;
        if (typeof callback === "function") {
            return callback();
        }
    }
};

NETDATA.sparklineChartUpdate = function (state, data) {
    state.sparkline_options.width = state.chartWidth();
    state.sparkline_options.height = state.chartHeight();

    $(state.element_chart).sparkline(data.result, state.sparkline_options);
    return true;
};

NETDATA.sparklineChartCreate = function (state, data) {
    let type = NETDATA.dataAttribute(state.element, 'sparkline-type', 'line');
    let lineColor = NETDATA.dataAttribute(state.element, 'sparkline-linecolor', state.chartCustomColors()[0]);
    let fillColor = NETDATA.dataAttribute(state.element, 'sparkline-fillcolor', ((state.chart.chart_type === 'line') ? NETDATA.themes.current.background : NETDATA.colorLuminance(lineColor, NETDATA.chartDefaults.fill_luminance)));
    let chartRangeMin = NETDATA.dataAttribute(state.element, 'sparkline-chartrangemin', undefined);
    let chartRangeMax = NETDATA.dataAttribute(state.element, 'sparkline-chartrangemax', undefined);
    let composite = NETDATA.dataAttribute(state.element, 'sparkline-composite', undefined);
    let enableTagOptions = NETDATA.dataAttribute(state.element, 'sparkline-enabletagoptions', undefined);
    let tagOptionPrefix = NETDATA.dataAttribute(state.element, 'sparkline-tagoptionprefix', undefined);
    let tagValuesAttribute = NETDATA.dataAttribute(state.element, 'sparkline-tagvaluesattribute', undefined);
    let disableHiddenCheck = NETDATA.dataAttribute(state.element, 'sparkline-disablehiddencheck', undefined);
    let defaultPixelsPerValue = NETDATA.dataAttribute(state.element, 'sparkline-defaultpixelspervalue', undefined);
    let spotColor = NETDATA.dataAttribute(state.element, 'sparkline-spotcolor', undefined);
    let minSpotColor = NETDATA.dataAttribute(state.element, 'sparkline-minspotcolor', undefined);
    let maxSpotColor = NETDATA.dataAttribute(state.element, 'sparkline-maxspotcolor', undefined);
    let spotRadius = NETDATA.dataAttribute(state.element, 'sparkline-spotradius', undefined);
    let valueSpots = NETDATA.dataAttribute(state.element, 'sparkline-valuespots', undefined);
    let highlightSpotColor = NETDATA.dataAttribute(state.element, 'sparkline-highlightspotcolor', undefined);
    let highlightLineColor = NETDATA.dataAttribute(state.element, 'sparkline-highlightlinecolor', undefined);
    let lineWidth = NETDATA.dataAttribute(state.element, 'sparkline-linewidth', undefined);
    let normalRangeMin = NETDATA.dataAttribute(state.element, 'sparkline-normalrangemin', undefined);
    let normalRangeMax = NETDATA.dataAttribute(state.element, 'sparkline-normalrangemax', undefined);
    let drawNormalOnTop = NETDATA.dataAttribute(state.element, 'sparkline-drawnormalontop', undefined);
    let xvalues = NETDATA.dataAttribute(state.element, 'sparkline-xvalues', undefined);
    let chartRangeClip = NETDATA.dataAttribute(state.element, 'sparkline-chartrangeclip', undefined);
    let chartRangeMinX = NETDATA.dataAttribute(state.element, 'sparkline-chartrangeminx', undefined);
    let chartRangeMaxX = NETDATA.dataAttribute(state.element, 'sparkline-chartrangemaxx', undefined);
    let disableInteraction = NETDATA.dataAttributeBoolean(state.element, 'sparkline-disableinteraction', false);
    let disableTooltips = NETDATA.dataAttributeBoolean(state.element, 'sparkline-disabletooltips', false);
    let disableHighlight = NETDATA.dataAttributeBoolean(state.element, 'sparkline-disablehighlight', false);
    let highlightLighten = NETDATA.dataAttribute(state.element, 'sparkline-highlightlighten', 1.4);
    let highlightColor = NETDATA.dataAttribute(state.element, 'sparkline-highlightcolor', undefined);
    let tooltipContainer = NETDATA.dataAttribute(state.element, 'sparkline-tooltipcontainer', undefined);
    let tooltipClassname = NETDATA.dataAttribute(state.element, 'sparkline-tooltipclassname', undefined);
    let tooltipFormat = NETDATA.dataAttribute(state.element, 'sparkline-tooltipformat', undefined);
    let tooltipPrefix = NETDATA.dataAttribute(state.element, 'sparkline-tooltipprefix', undefined);
    let tooltipSuffix = NETDATA.dataAttribute(state.element, 'sparkline-tooltipsuffix', ' ' + state.units_current);
    let tooltipSkipNull = NETDATA.dataAttributeBoolean(state.element, 'sparkline-tooltipskipnull', true);
    let tooltipValueLookups = NETDATA.dataAttribute(state.element, 'sparkline-tooltipvaluelookups', undefined);
    let tooltipFormatFieldlist = NETDATA.dataAttribute(state.element, 'sparkline-tooltipformatfieldlist', undefined);
    let tooltipFormatFieldlistKey = NETDATA.dataAttribute(state.element, 'sparkline-tooltipformatfieldlistkey', undefined);
    let numberFormatter = NETDATA.dataAttribute(state.element, 'sparkline-numberformatter', function (n) {
        return n.toFixed(2);
    });
    let numberDigitGroupSep = NETDATA.dataAttribute(state.element, 'sparkline-numberdigitgroupsep', undefined);
    let numberDecimalMark = NETDATA.dataAttribute(state.element, 'sparkline-numberdecimalmark', undefined);
    let numberDigitGroupCount = NETDATA.dataAttribute(state.element, 'sparkline-numberdigitgroupcount', undefined);
    let animatedZooms = NETDATA.dataAttributeBoolean(state.element, 'sparkline-animatedzooms', false);

    if (spotColor === 'disable') {
        spotColor = '';
    }
    if (minSpotColor === 'disable') {
        minSpotColor = '';
    }
    if (maxSpotColor === 'disable') {
        maxSpotColor = '';
    }

    // state.log('sparkline type ' + type + ', lineColor: ' + lineColor + ', fillColor: ' + fillColor);

    state.sparkline_options = {
        type: type,
        lineColor: lineColor,
        fillColor: fillColor,
        chartRangeMin: chartRangeMin,
        chartRangeMax: chartRangeMax,
        composite: composite,
        enableTagOptions: enableTagOptions,
        tagOptionPrefix: tagOptionPrefix,
        tagValuesAttribute: tagValuesAttribute,
        disableHiddenCheck: disableHiddenCheck,
        defaultPixelsPerValue: defaultPixelsPerValue,
        spotColor: spotColor,
        minSpotColor: minSpotColor,
        maxSpotColor: maxSpotColor,
        spotRadius: spotRadius,
        valueSpots: valueSpots,
        highlightSpotColor: highlightSpotColor,
        highlightLineColor: highlightLineColor,
        lineWidth: lineWidth,
        normalRangeMin: normalRangeMin,
        normalRangeMax: normalRangeMax,
        drawNormalOnTop: drawNormalOnTop,
        xvalues: xvalues,
        chartRangeClip: chartRangeClip,
        chartRangeMinX: chartRangeMinX,
        chartRangeMaxX: chartRangeMaxX,
        disableInteraction: disableInteraction,
        disableTooltips: disableTooltips,
        disableHighlight: disableHighlight,
        highlightLighten: highlightLighten,
        highlightColor: highlightColor,
        tooltipContainer: tooltipContainer,
        tooltipClassname: tooltipClassname,
        tooltipChartTitle: state.title,
        tooltipFormat: tooltipFormat,
        tooltipPrefix: tooltipPrefix,
        tooltipSuffix: tooltipSuffix,
        tooltipSkipNull: tooltipSkipNull,
        tooltipValueLookups: tooltipValueLookups,
        tooltipFormatFieldlist: tooltipFormatFieldlist,
        tooltipFormatFieldlistKey: tooltipFormatFieldlistKey,
        numberFormatter: numberFormatter,
        numberDigitGroupSep: numberDigitGroupSep,
        numberDecimalMark: numberDecimalMark,
        numberDigitGroupCount: numberDigitGroupCount,
        animatedZooms: animatedZooms,
        width: state.chartWidth(),
        height: state.chartHeight()
    };

    $(state.element_chart).sparkline(data.result, state.sparkline_options);

    return true;
};
