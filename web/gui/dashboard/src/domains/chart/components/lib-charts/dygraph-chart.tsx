import { findIndex } from "ramda"
import React, {
  useLayoutEffect, useRef, useState, useCallback,
} from "react"
import { useSelector } from "react-redux"
import classNames from "classnames"
import Dygraph from "dygraphs"
import "dygraphs/src/extras/smooth-plotter"

import { NetdataDygraph } from "types/vendor-overrides"
import { useDateTime } from "utils/date-time"
import { selectGlobalSelectionMaster } from "domains/global/selectors"

import { Attributes } from "../../utils/transformDataAttributes"
import {
  chartLibrariesSettings,
  ChartLibraryConfig,
  ChartLibraryName,
} from "../../utils/chartLibrariesSettings"
import { ChartData, ChartDetails } from "../../chart-types"

import "./dygraph-chart.css"

// all noops are just todos, places to not break linter
const noop: any = () => {}

interface GetInitialDygraphOptions {
  attributes: Attributes,
  chartData: ChartData,
  chartDetails: ChartDetails,
  chartSettings: ChartLibraryConfig,
  dimensionsVisibility: boolean[]
  hiddenLabelsElementId: string,
  orderedColors: string[],
  setHoveredX: (hoveredX: number | null) => void
  unitsCurrent: string,
  xAxisTimeString: (d: Date) => string,
}
const getInitialDygraphOptions = ({
  attributes,
  chartData,
  chartDetails,
  chartSettings,
  dimensionsVisibility,
  hiddenLabelsElementId,
  orderedColors,
  setHoveredX,
  unitsCurrent,
  xAxisTimeString,
}: GetInitialDygraphOptions) => {
  const isSparkline = attributes.dygraphTheme === "sparkline"
  const highlightCircleSize = isSparkline ? 3 : 4

  const isLogScale = (chartSettings.isLogScale as ((a: Attributes) => boolean))(attributes)
  const {
    dygraphType: dygraphRequestedType = chartDetails.chart_type,
  } = attributes
  // corresponds to state.tmp.dygraph_chart_type in old app
  let dygraphChartType = dygraphRequestedType
  if (dygraphChartType === "stacked" && chartData.dimensions === 1) {
    dygraphChartType = "area"
  }
  if (dygraphChartType === "stacked" && isLogScale) {
    dygraphChartType = "area"
  }
  const {
    dygraphSmooth = dygraphChartType === "line"
      && !isSparkline,
    dygraphDrawAxis = true,
  } = attributes
  const {
    // destructuring with default values
    dygraphColors = orderedColors,
    dygraphRightGap = 5,
    dygraphShowRangeSelector = false,
    dygraphShowRoller = false,
    dygraphTitle = attributes.title || chartDetails.title,
    dygraphTitleHeight = 19,
    dygraphLegend = "always",
    dygraphLabelsDiv = hiddenLabelsElementId,
    dygraphLabelsSeparateLine = true,
    dygraphIncludeZero = dygraphChartType === "stacked",
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
      : (dygraphSmooth === true
        ? 1.5
        : 0.7),

    dygraphStrokePattern,
    dygraphDrawPoints = false,
    dygraphDrawGapEdgePoints = true,
    dygraphConnectSeparatedPoints = false,
    dygraphPointSize = 1,
    dygraphStepPlot = false,
    dygraphStrokeBorderColor = window.NETDATA.themes.current.background,
    dygraphStrokeBorderWidth = 0,
    dygraphFillGraph = (dygraphChartType === "area" || dygraphChartType === "stacked"),
    dygraphFillAlpha = dygraphChartType === "stacked"
      ? window.NETDATA.options.current.color_fill_opacity_stacked
      : window.NETDATA.options.current.color_fill_opacity_area,
    dygraphStackedGraph = dygraphChartType === "stacked",
    dygraphStackedGraphNanFill = "none",
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

    dygraphXPixelsPerLabel = 50,
    dygraphXAxisLabelWidth = 60,
    dygraphDrawXAxis = dygraphDrawAxis,
    dygraphYPixelsPerLabel = 15,
    dygraphYAxisLabelWidth = 50,
    dygraphDrawYAxis = dygraphDrawAxis,
  } = attributes
  return {
    colors: dygraphColors,

    // leave a few pixels empty on the right of the chart
    rightGap: dygraphRightGap,
    showRangeSelector: dygraphShowRangeSelector,
    showRoller: dygraphShowRoller,
    title: dygraphTitle,
    titleHeight: dygraphTitleHeight,
    legend: dygraphLegend, // we need this to get selection events
    labels: chartData.result.labels,
    labelsDiv: dygraphLabelsDiv,

    labelsSeparateLines: dygraphLabelsSeparateLine,
    labelsShowZeroValues: isLogScale ? false : dygraphShowZeroValues,
    labelsKMB: false,
    labelsKMG2: false,
    showLabelsOnHighlight: dygraphShowLabelsOnHighLight,
    hideOverlayOnMouseOut: dygraphHideOverlayOnMouseOut,
    includeZero: dygraphIncludeZero,
    xRangePad: dygraphXRangePad,
    yRangePad: dygraphYRangePad,
    valueRange: dygraphValueRange,
    ylabel: unitsCurrent,
    yLabelWidth: dygraphYLabelWidth,

    // the function to plot the chart
    plotter: (dygraphSmooth && window.NETDATA.options.current) ? window.smoothPlotter : null,

    // The width of the lines connecting data points.
    // This can be used to increase the contrast or some graphs.
    strokeWidth: dygraphStrokeWidth,
    strokePattern: dygraphStrokePattern,

    // The size of the dot to draw on each point in pixels (see drawPoints).
    // A dot is always drawn when a point is "isolated",
    // i.e. there is a missing point on either side of it.
    // This also controls the size of those dots.
    drawPoints: dygraphDrawPoints,

    // Draw points at the edges of gaps in the data.
    // This improves visibility of small data segments or other data irregularities.
    drawGapEdgePoints: dygraphDrawGapEdgePoints,
    connectSeparatedPoints: isLogScale ? false : dygraphConnectSeparatedPoints,
    pointSize: dygraphPointSize,

    // enabling this makes the chart with little square lines
    stepPlot: dygraphStepPlot,

    // Draw a border around graph lines to make crossing lines more easily
    // distinguishable. Useful for graphs with many lines.
    strokeBorderColor: dygraphStrokeBorderColor,
    strokeBorderWidth: dygraphStrokeBorderWidth,
    fillGraph: dygraphFillGraph,
    fillAlpha: dygraphFillAlpha,
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
    highlightCircleSize: dygraphHighlighCircleSize,
    highlightSeriesOpts: dygraphHighlightSeriesOpts, // TOO SLOW: { strokeWidth: 1.5 },
    // TOO SLOW: (state.tmp.dygraph_chart_type === 'stacked')?0.7:0.5,
    highlightSeriesBackgroundAlpha: dygraphHighlightSeriesBackgroundAlpha,
    visibility: dimensionsVisibility,
    logscale: isLogScale,

    axes: {
      x: {
        pixelsPerLabel: dygraphXPixelsPerLabel,
        // unsufficient typings for Dygraph
        // @ts-ignore
        ticker: Dygraph.dateTicker,
        axisLabelWidth: dygraphXAxisLabelWidth,
        drawAxis: dygraphDrawXAxis,
        axisLabelFormatter: (d: Date | number) => xAxisTimeString(d as Date),
      },
      y: {
        logscale: isLogScale,
        pixelsPerLabel: dygraphYPixelsPerLabel,
        axisLabelWidth: dygraphYAxisLabelWidth,
        drawAxis: dygraphDrawYAxis,
        // axisLabelFormatter is added on the updates
      },
    },

    highlightCallback(
      event: MouseEvent, xval: number,
    ) {
      // todo
      // state.pauseChart()

      // todo dont know if that's still valid (following comment is from old code)
      // there is a bug in dygraph when the chart is zoomed enough
      // the time it thinks is selected is wrong
      // here we calculate the time t based on the row number selected
      // which is ok
      // let t = state.data_after + row * state.data_update_every;
      // console.log('row = ' + row + ', x = ' + x + ', t = ' + t + ' ' + ((t === x)?'SAME':(
      // Math.abs(x-t)<=state.data_update_every)?'SIMILAR':'DIFFERENT') + ', rows in db: ' +
      // state.data_points + ' visible(x) = ' + state.timeIsVisible(x) + ' visible(t) = ' +
      // state.timeIsVisible(t) + ' r(x) = ' + state.calculateRowForTime(x) + ' r(t) = ' +
      // state.calculateRowForTime(t) + ' range: ' + state.data_after + ' - ' + state.data_before +
      // ' real: ' + state.data.after + ' - ' + state.data.before + ' every: ' +
      // state.data_update_every);

      // todo
      // if (state.tmp.dygraph_mouse_down !== true) {
      setHoveredX(xval)
      // }
    },

    unhighlightCallback() {
      // todo
      // if (state.tmp.dygraph_mouse_down) {
      //   return;
      // }

      // todo
      // state.unpauseChart();
      setHoveredX(null)
    },
  }
}

interface Props {
  attributes: Attributes
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: ChartLibraryName
  chartUuid: string
  colors: {
    [key: string]: string
  }
  dimensionsVisibility: boolean[]
  legendFormatValue: ((v: number) => number | string) | undefined
  orderedColors: string[]

  hoveredX: number | null
  setHoveredX: (hoveredX: number | null) => void
  setMinMax: (minMax: [number, number]) => void
  unitsCurrent: string
  updateChartPanAndZoom: (arg: { after: number, before: number, masterID: string }) => void
}
export const DygraphChart = ({
  attributes,
  chartData,
  chartDetails,
  chartLibrary,
  // colors,
  chartUuid,
  dimensionsVisibility,
  legendFormatValue,
  orderedColors,

  hoveredX,
  setHoveredX,
  setMinMax,
  unitsCurrent,
  updateChartPanAndZoom,
}: Props) => {
  const { xAxisTimeString } = useDateTime()
  const chartSettings = chartLibrariesSettings[chartLibrary]
  const hiddenLabelsElementId = `${chartUuid}-hidden-labels-id`

  const chartElement = useRef<HTMLDivElement>(null)
  const [dygraphInstance, setDygraphInstance] = useState<Dygraph | null>(null)

  const updateChartPanOrZoom = useCallback(({ after, before }) => {
    updateChartPanAndZoom({
      after,
      before,
      masterID: chartUuid,
    })
  }, [chartUuid, updateChartPanAndZoom])

  const latestIsUserAction = useRef(false)

  useLayoutEffect(() => {
    if (chartElement && chartElement.current && !dygraphInstance) {
      const dygraphOptionsStatic = getInitialDygraphOptions({
        attributes,
        chartData,
        chartDetails,
        chartSettings,
        dimensionsVisibility,
        hiddenLabelsElementId,
        orderedColors,
        setHoveredX,
        unitsCurrent,
        xAxisTimeString,
      })

      latestIsUserAction.current = true

      const dygraphOptions = {
        ...dygraphOptionsStatic,
        drawCallback(dygraph: Dygraph) {
          // the user has panned the chart and this is called to re-draw the chart
          // 1. refresh this chart by adding data to it
          // 2. notify all the other charts about the update they need

          // to prevent an infinite loop (feedback), we use
          //     state.tmp.dygraph_user_action
          // - when true, this is initiated by a user
          // - when false, this is feedback

          if (latestIsUserAction.current) {
            latestIsUserAction.current = false
            const xRange = dygraph.xAxisRange()
            const after = Math.round(xRange[0])
            const before = Math.round(xRange[1])

            // if (before <= netdataLast && after >= netdataFirst) {
            // todo update only when we are within the data limits
            updateChartPanOrZoom({ after, before })
            // }
          }
        },
        interactionModel: {
          mousedown(event: MouseEvent, dygraph: Dygraph, context: any) {
            context.initializeMouseDown(event, dygraph, context)

            if (event.button && event.button === 1) {
              if (event.shiftKey) {
                noop()
              } else if (event.altKey || event.ctrlKey || event.metaKey) {
                noop()
              } else {
                noop()
              }
            } else if (event.shiftKey) {
              noop()
            } else if (event.altKey || event.ctrlKey || event.metaKey) {
              noop()
            } else {
              latestIsUserAction.current = true

              // @ts-ignore
              Dygraph.startPan(event, dygraph, context)
            }
          },
          mousemove(event: MouseEvent, dygraph: Dygraph, context: any) {
            // if (state.tmp.dygraph_highlight_after !== null) {
            // else if (
            if (context.isPanning) {
              latestIsUserAction.current = true
              // eslint-disable-next-line no-param-reassign
              context.is2DPan = false
              // @ts-ignore
              Dygraph.movePan(event, dygraph, context)
            }
          },
          mouseup(event: MouseEvent, dygraph: Dygraph, context: any) {
            if (context.isPanning) {
              latestIsUserAction.current = true
              // @ts-ignore
              Dygraph.endPan(event, dygraph, context)
            }
          },
          click(event: MouseEvent, dygraph: Dygraph, context: any) {
            event.preventDefault()
            noop(dygraph, context)
          },
          touchend(event: MouseEvent, dygraph: Dygraph, context: any) {
            noop(event, dygraph, context)
          },
        },
      }

      // todo if any flickering will happen, show the dygraph chart only when it's
      // updated with proper formatting (toggle visibility with css)
      const instance = new Dygraph((chartElement.current), chartData.result.data, dygraphOptions)
      setDygraphInstance(instance)

      const extremes = (instance as NetdataDygraph).yAxisExtremes()[0]
      setMinMax(extremes)
    }
  }, [attributes, chartData, chartDetails, chartSettings, chartUuid, dimensionsVisibility,
    dygraphInstance, hiddenLabelsElementId, orderedColors, setHoveredX, setMinMax,
    unitsCurrent, updateChartPanOrZoom, xAxisTimeString])

  useLayoutEffect(() => {
    if (dygraphInstance && legendFormatValue) {
      const isSparkline = attributes.dygraphTheme === "sparkline"
      // corresponds to NETDATA.dygraphChartUpdate in old dashboard
      dygraphInstance.updateOptions({
        axes: {
          y: {
            axisLabelFormatter: (y: Date | number) => legendFormatValue(y as number),
          },
        },
        visibility: dimensionsVisibility,
        ylabel: isSparkline ? unitsCurrent : undefined,
      })
    }
  }, [attributes.dygraphTheme, dimensionsVisibility, dygraphInstance, legendFormatValue,
    unitsCurrent])


  // update data of the chart
  useLayoutEffect(() => {
    if (dygraphInstance) {
      dygraphInstance.updateOptions({
        file: chartData.result.data,
      })
    }
  }, [chartData.result.data, dygraphInstance])


  // set selection
  const currentSelectionMasterId = useSelector(selectGlobalSelectionMaster)
  useLayoutEffect(() => {
    if (dygraphInstance && currentSelectionMasterId && currentSelectionMasterId !== chartUuid) {
      const hoveredRow = findIndex((x: any) => x[0] === hoveredX, chartData.result.data)

      if (hoveredX === null) {
        dygraphInstance.clearSelection()
        return
      }
      dygraphInstance.setSelection(hoveredRow)
    }
  }, [chartData.result.data, chartUuid, currentSelectionMasterId, dygraphInstance, hoveredX])


  const chartElemId = `${chartLibrary}-${chartUuid}-chart`
  const { hasLegend } = chartSettings
  return (
    <>
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
          )}
      />
      <div className="dygraph-chart__labels-hidden" id={hiddenLabelsElementId} />
    </>
  )
}
