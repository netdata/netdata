/* eslint-disable max-len */
import React, { useLayoutEffect, useRef } from "react"
import classNames from "classnames"
import Dygraph from "dygraphs"

import { Attributes } from "../../utils/transformDataAttributes"
import { chartLibrariesSettings, ChartLibraryName } from "../../utils/chartLibrariesSettings"
import { ChartData, ChartDetails } from "../../chart-types"

const getDygraphOptions = (
  attributes: Attributes,
  orderedColors: string[],
) => {
  const dygraphChartType = "stacked" // todo
  const isSparkline = true // todo
  const highlightCircleSize = isSparkline ? 3 : 4
  // let smooth = NETDATA.dygraph.smooth
  //   ? (NETDATA.dataAttributeBoolean(state.element, 'dygraph-smooth', (state.tmp.dygraph_chart_type === 'line' && NETDATA.chartLibraries.dygraph.isSparkline(state) === false)))
  //   : false;
  const smooth: boolean = false
  const {
    dygraphColors = orderedColors,
    dygraphRightGap = 5,
    dygraphShowRangeSelector = false,
    dygraphShowRoller = false,
    dygraphTitle = "dygraph title", // todo get from state.title
    dygraphTitleHeight = 19,
    dygraphLegend = "always",
    // dygraphLabelsDiv = "labelsdiv", // todo state.element_legend_childs.hidden
    dygraphLabelsDiv, // todo state.element_legend_childs.hidden
    dygraphLabelsSeparateLine = true,
    dygraphShowZeroValues = true,
    dygraphShowLabelsOnHighLight = true,
    dygraphHideOverlayOnMouseOut = true,
    dygraphXRangePad = 0,
    dygraphYRangePad = 1,
    dygraphValueRange = [null, null],
    dygraphYLabelWidth = 12,
    // eslint-disable-next-line no-nested-ternary
    dygraphStrokeWidth = dygraphChartType === "stacked"
      ? 0.1
      // @ts-ignore
      : (smooth === true
        ? 1.5
        : 0.7),

    dygraphStrokePattern,
    // dygraphDrawPoints = false,
    dygraphDrawGapEdgePoints = true,
    dygraphConnectSeparatedPoints = false, // todo
    dygraphPointSize = 1,
    dygraphStepPlot = false,
    dygraphStrokeBorderColor = window.NETDATA.themes.current.background,
    // dygraphStrokeBorderWidth, // todo
    // dygraphFillGraph, // todo
    // dygraphFillAlpha // todo
    dygraphStackedGraph = dygraphChartType === "stacked",
    dygraphStackedGraphNanFill = "none",
    dygraphDrawAxis = true,
    dygraphAxisLabelFontSize = 10,
    dygraphAxisLineColor = window.NETDATA.themes.current.axis,
    dygraphAxisLineWidth = 1.0,
    dygraphDrawGrid = true,
    dygraphGridLinePattern,
    dygraphGridLineWidth = 1.0,
    dygraphGridLineColor = window.NETDATA.themes.current.grid,
    dygraphMaxNumberWidth = 8,
    dygraphSigFigs,
    dygraphDigitsAfterDecimal = 2,
    dygraphHighlighCircleSize = highlightCircleSize,
    dygraphHighlightSeriesOpts,
    dygraphHighlightSeriesBackgroundAlpha,
  } = attributes
  const isLogScale = false // todo
  const includeZero = true // todo
  const yLabel = "yLabel" // todo (state.unts_current)
  return {
    colors: dygraphColors,

    // leave a few pixels empty on the right of the chart
    rightGap: dygraphRightGap,
    showRangeSelector: dygraphShowRangeSelector,
    showRoller: dygraphShowRoller,
    title: dygraphTitle,
    titleHeight: dygraphTitleHeight,
    legend: dygraphLegend, // we need this to get selection events
    // labels: data.result.labels, // todo
    labelsDiv: dygraphLabelsDiv,

    labelsSeparateLines: dygraphLabelsSeparateLine,
    labelsShowZeroValues: isLogScale ? false : dygraphShowZeroValues,
    labelsKMB: false,
    labelsKMG2: false,
    showLabelsOnHighlight: dygraphShowLabelsOnHighLight,
    hideOverlayOnMouseOut: dygraphHideOverlayOnMouseOut,
    includeZero,
    xRangePad: dygraphXRangePad,
    yRangePad: dygraphYRangePad,
    valueRange: dygraphValueRange,
    ylabel: yLabel,
    yLabelWidth: dygraphYLabelWidth,

    // the function to plot the chart
    plotter: null,

    // The width of the lines connecting data points.
    // This can be used to increase the contrast or some graphs.

    strokeWidth: dygraphStrokeWidth,
    strokePattern: dygraphStrokePattern,

    // The size of the dot to draw on each point in pixels (see drawPoints).
    // A dot is always drawn when a point is "isolated",
    // i.e. there is a missing point on either side of it.
    // This also controls the size of those dots.
    // drawPoints: dygraphDrawPoints,

    // Draw points at the edges of gaps in the data.
    // This improves visibility of small data segments or other data irregularities.
    drawGapEdgePoints: dygraphDrawGapEdgePoints,

    // todo
    connectSeparatedPoints: isLogScale
      ? false
      : dygraphConnectSeparatedPoints,

    pointSize: dygraphPointSize,

    // enabling this makes the chart with little square lines
    stepPlot: dygraphStepPlot,

    // Draw a border around graph lines to make crossing lines more easily
    // distinguishable. Useful for graphs with many lines.
    strokeBorderColor: dygraphStrokeBorderColor,

    // todo
    // strokeBorderWidth: NETDATA.dataAttribute(state.element, 'dygraph-strokeborderwidth', (state.tmp.dygraph_chart_type === 'stacked') ? 0.0 : 0.0),
    // todo
    // fillGraph: NETDATA.dataAttribute(state.element, 'dygraph-fillgraph', (state.tmp.dygraph_chart_type === 'area' || state.tmp.dygraph_chart_type === 'stacked')),
    // fillAlpha: NETDATA.dataAttribute(state.element, 'dygraph-fillalpha',
    //   ((state.tmp.dygraph_chart_type === 'stacked')
    //     ? NETDATA.options.current.color_fill_opacity_stacked
    //     : NETDATA.options.current.color_fill_opacity_area)
    // ),
    stackedGraph: dygraphStackedGraph,
    stackedGraphNaNFill: dygraphStackedGraphNanFill,
    drawAxis: dygraphDrawAxis,
    axisLabelFontSize: dygraphAxisLabelFontSize,
    axisLineColor: dygraphAxisLineColor,
    axisLineWidth: dygraphAxisLineWidth,
    drawGrid: dygraphDrawGrid,
    gridLinePattern: dygraphGridLinePattern,
    gridLineWidth: dygraphGridLineWidth,
    gridLineColor: dygraphGridLineColor,
    maxNumberWidth: dygraphMaxNumberWidth,
    sigFigs: dygraphSigFigs,
    digitsAfterDecimal: dygraphDigitsAfterDecimal,
    // removed (because it's a function)
    // valueFormatter: NETDATA.dataAttribute(state.element, 'dygraph-valueformatter', undefined),
    highlightCircleSize: dygraphHighlighCircleSize,
    highlightSeriesOpts: dygraphHighlightSeriesOpts, // TOO SLOW: { strokeWidth: 1.5 },
    // TOO SLOW: (state.tmp.dygraph_chart_type === 'stacked')?0.7:0.5,
    highlightSeriesBackgroundAlpha: dygraphHighlightSeriesBackgroundAlpha,
    // removed (because it's a function)
    // pointClickCallback: NETDATA.dataAttribute(state.element, 'dygraph-pointclickcallback', undefined),
    // todo
    // visibility: state.dimensions_visibility.selected2BooleanArray(state.data.dimension_names),
    // there was a bug previously, value "y" was used instead of true // todo check that
    logscale: isLogScale,
  }
}

interface Props {
  attributes: Attributes
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: ChartLibraryName
  colors: {
    [key: string]: string
  }
  chartUuid: string
  orderedColors: string[]
}
export const DygraphChart = ({
  attributes,
  chartData,
  // chartDetails,
  chartLibrary,
  // colors,
  chartUuid,
  orderedColors,
}: Props) => {
  const chartElement = useRef<HTMLDivElement>(null)

  const dygraphOptions = getDygraphOptions(attributes, orderedColors)

  useLayoutEffect(() => {
    if (chartElement && chartElement.current) {
      // eslint-disable-next-line no-new
      new Dygraph((chartElement.current), chartData.result.data, dygraphOptions)
    }
  }, [chartData.result.data, dygraphOptions])
  const chartElemId = `${chartLibrary}-${chartUuid}-chart`
  const chartSettings = chartLibrariesSettings[chartLibrary]
  const { hasLegend } = chartSettings
  return (
    <div
      ref={chartElement}
      id={chartElemId}
      className={hasLegend
        ? classNames(
          "netdata-chart-with-legend-right",
          `netdata-${chartLibrary}-chart-with-legend-right`,
        )
        : classNames(
          "netdata-chart",
          `netdata-${chartLibrary}-chart`,
        )
      }
    />
  )
}
