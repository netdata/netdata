import { sortBy } from "ramda"
import React, {
  useLayoutEffect, useRef, useCallback,
} from "react"
import { useSelector } from "react-redux"
import classNames from "classnames"
import Dygraph from "dygraphs"
import "dygraphs/src-es5/extras/smooth-plotter"

import { DygraphArea, NetdataDygraph } from "types/vendor-overrides"
import { useDateTime } from "utils/date-time"
import { selectGlobalChartUnderlay, selectGlobalSelectionMaster } from "domains/global/selectors"

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
  isRemotelyControlled: boolean
  legendFormatValue: ((v: number) => number | string) | undefined
  onUpdateChartPanAndZoom: (arg: {
    after: number, before: number,
    callback: (after: number, before: number) => void,
    masterID: string,
    shouldNotExceedAvailableRange: boolean,
  }) => void
  orderedColors: string[]

  hoveredX: number | null
  setGlobalChartUnderlay: (arg: { after: number, before: number, masterID: string }) => void
  setHoveredX: (hoveredX: number | null, noMaster?: boolean) => void
  setMinMax: (minMax: [number, number]) => void
  unitsCurrent: string
  viewAfter: number
  viewBefore: number
}
export const DygraphChart = ({
  attributes,
  chartData,
  chartDetails,
  chartLibrary,
  // colors,
  chartUuid,
  dimensionsVisibility,
  isRemotelyControlled,
  legendFormatValue,
  onUpdateChartPanAndZoom,
  orderedColors,

  hoveredX,
  setGlobalChartUnderlay,
  setHoveredX,
  setMinMax,
  unitsCurrent,
  viewAfter,
  viewBefore,
}: Props) => {
  // setGlobalChartUnderlay is using state from closure (chartData.after), so we need to have always
  // the newest callback. Unfortunately we cannot use Dygraph.updateOptions() (library restriction)
  // for interactionModel callbacks so we need to keep the callback in mutable ref
  const propsRef = useRef({
    hoveredX,
    setGlobalChartUnderlay,
    viewAfter,
    viewBefore,
  })
  useLayoutEffect(() => {
    propsRef.current.hoveredX = hoveredX
    propsRef.current.setGlobalChartUnderlay = setGlobalChartUnderlay
    propsRef.current.viewAfter = viewAfter
    propsRef.current.viewBefore = viewBefore
  }, [hoveredX, setGlobalChartUnderlay, viewAfter, viewBefore])

  const { xAxisTimeString } = useDateTime()
  const chartSettings = chartLibrariesSettings[chartLibrary]
  const hiddenLabelsElementId = `${chartUuid}-hidden-labels-id`

  const chartElement = useRef<HTMLDivElement>(null)
  // React.useState. so it can be mocked in test
  const [dygraphInstance, setDygraphInstance] = React.useState<Dygraph | null>(null)

  const updateChartPanOrZoom = useCallback(({
    after, before,
    callback,
    shouldNotExceedAvailableRange,
  }) => {
    onUpdateChartPanAndZoom({
      after,
      before,
      callback,
      masterID: chartUuid,
      shouldNotExceedAvailableRange,
    })
  }, [chartUuid, onUpdateChartPanAndZoom])
  const globalChartUnderlay = useSelector(selectGlobalChartUnderlay)

  // state.tmp.dygraph_user_action in old dashboard
  const latestIsUserAction = useRef(false)
  // state.tmp.dygraph_mouse_down in old dashboard
  const isMouseDown = useRef(false)
  // state.tmp.dygraph_highlight_after in old dashboard
  const dygraphHighlightAfter = useRef<null | number>(null)
  // state.dygraph_last_touch_move in old dashboard
  const dygraphLastTouchMove = useRef(0)
  // state.dygraph_last_touch_page_x in old dashboard
  const dygraphLastTouchPageX = useRef(0)
  // state.dygraph_last_touch_end in old dashboard
  const dygraphLastTouchEnd = useRef<undefined | number>()

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
        unitsCurrent,
        xAxisTimeString,
      })

      latestIsUserAction.current = false

      const dygraphOptions = {
        ...dygraphOptionsStatic,

        highlightCallback(
          event: MouseEvent, xval: number,
        ) {
          // todo
          // state.pauseChart()

          const newHoveredX = isMouseDown.current
            ? null
            : xval

          const currentHoveredX = propsRef.current.hoveredX
          if (newHoveredX !== currentHoveredX) {
            setHoveredX(newHoveredX)
          }
        },

        unhighlightCallback() {
          // todo
          // state.unpauseChart();
          if (propsRef.current.hoveredX !== null) {
            setHoveredX(null)
          }
        },
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

            if (before <= (chartData.last_entry * 1000)
              && after >= (chartData.first_entry * 1000)
            ) {
              updateChartPanOrZoom({ after, before })
            }
          }
        },
        zoomCallback: (minDate: number, maxDate: number) => {
          latestIsUserAction.current = true
          updateChartPanOrZoom({ after: minDate, before: maxDate })
        },

        // interactionModel cannot be replaced with updateOptions(). we need to keep all changing
        // values and callbacks in mutable ref,
        interactionModel: {
          mousedown(event: MouseEvent, dygraph: Dygraph, context: any) {
            // Right-click should not initiate anything.
            if (event.button && event.button === 2) {
              return
            }

            latestIsUserAction.current = true
            isMouseDown.current = true
            context.initializeMouseDown(event, dygraph, context)

            if (event.button && event.button === 1) {
              // middle mouse button

              if (event.shiftKey) {
                // panning
                dygraphHighlightAfter.current = null
                // @ts-ignore
                Dygraph.startPan(event, dygraph, context)

              } else if (event.altKey || event.ctrlKey || event.metaKey) {
                // middle mouse button highlight
                dygraphHighlightAfter.current = dygraph.toDataXCoord(event.offsetX)
                // @ts-ignore
                Dygraph.startZoom(event, dygraph, context)

              } else {
                // middle mouse button selection for zoom
                dygraphHighlightAfter.current = null
                // @ts-ignore
                Dygraph.startZoom(event, dygraph, context)
              }

            } else if (event.shiftKey) {
              // left mouse button selection for zoom (ZOOM)
              dygraphHighlightAfter.current = null
              // @ts-ignore
              Dygraph.startZoom(event, dygraph, context)

            } else if (event.altKey || event.ctrlKey || event.metaKey) {
              // left mouse button highlight
              dygraphHighlightAfter.current = dygraph.toDataXCoord(event.offsetX)
              // @ts-ignore
              Dygraph.startZoom(event, dygraph, context)

            } else {
              // left mouse button dragging (PAN)
              dygraphHighlightAfter.current = null
              // @ts-ignore
              Dygraph.startPan(event, dygraph, context)
            }
          },

          mousemove(event: MouseEvent, dygraph: Dygraph, context: any) {
            // if (state.tmp.dygraph_highlight_after !== null) {
            // else if (
            if (dygraphHighlightAfter.current !== null) {
              // highlight selection
              latestIsUserAction.current = true
              // @ts-ignore
              Dygraph.moveZoom(event, dygraph, context)
              event.preventDefault()

            } else if (context.isPanning) {
              latestIsUserAction.current = true
              // eslint-disable-next-line no-param-reassign
              context.is2DPan = false
              // @ts-ignore
              Dygraph.movePan(event, dygraph, context)

            } else if (context.isZooming) {
              // @ts-ignore
              Dygraph.moveZoom(event, dygraph, context)
            }
          },

          mouseup(event: MouseEvent, dygraph: Dygraph, context: any) {
            isMouseDown.current = false
            if (dygraphHighlightAfter.current !== null) {
              const sortedRange = sortBy((x) => +x, [
                dygraphHighlightAfter.current,
                dygraph.toDataXCoord(event.offsetX),
              ])

              propsRef.current.setGlobalChartUnderlay({
                after: sortedRange[0],
                before: sortedRange[1],
                masterID: chartUuid,
              })
              dygraphHighlightAfter.current = null
              // eslint-disable-next-line no-param-reassign
              context.isZooming = false

              // old dashboard code
              // @ts-ignore
              // eslint-disable-next-line no-underscore-dangle
              dygraph.clearZoomRect_()
              // this call probably fixes the broken selection circle during highlighting
              // @ts-ignore
              // eslint-disable-next-line no-underscore-dangle
              dygraph.drawGraph_(false)

            } else if (context.isPanning) {
              latestIsUserAction.current = true
              // @ts-ignore
              Dygraph.endPan(event, dygraph, context)

            } else if (context.isZooming) {
              latestIsUserAction.current = true
              // @ts-ignore
              Dygraph.endZoom(event, dygraph, context)
            }
          },

          wheel(event: WheelEvent, dygraph: Dygraph) {
            // Take the offset of a mouse event on the dygraph canvas and
            // convert it to a pair of percentages from the bottom left.
            // (Not top left, bottom is where the lower value is.)
            function offsetToPercentage(g: Dygraph, offsetX: number, offsetY: number) {
              // This is calculating the pixel offset of the leftmost date.
              const xOffset = g.toDomXCoord(g.xAxisRange()[0])
              const yar0 = g.yAxisRange(0)

              // This is calculating the pixel of the highest value. (Top pixel)
              const yOffset = g.toDomYCoord(yar0[1])

              // x y w and h are relative to the corner of the drawing area,
              // so that the upper corner of the drawing area is (0, 0).
              const x = offsetX - xOffset
              const y = offsetY - yOffset

              // This is computing the rightmost pixel, effectively defining the
              // width.
              // const w = g.toDomCoords(g.xAxisRange()[1], null)[0] - xOffset
              const w = g.toDomXCoord(g.xAxisRange()[1]) - xOffset

              // This is computing the lowest pixel, effectively defining the height.
              // const h = g.toDomCoords(null, yar0[0])[1] - yOffset
              const h = g.toDomYCoord(yar0[0]) - yOffset

              // Percentage from the left.
              const xPct = w === 0 ? 0 : (x / w)
              // Percentage from the top.
              const yPct = h === 0 ? 0 : (y / h)

              // The (1-) part below changes it from "% distance down from the top"
              // to "% distance up from the bottom".
              return [xPct, (1 - yPct)]
            }

            function adjustAxis(axis: [number, number], zoomInPercentage: number, bias: number) {
              const delta = axis[1] - axis[0]
              const increment = delta * zoomInPercentage
              const foo = [increment * bias, increment * (1 - bias)]

              return [axis[0] + foo[0], axis[1] - foo[1]]
            }

            // Adjusts [x, y] toward each other by zoomInPercentage%
            // Split it so the left/bottom axis gets xBias/yBias of that change and
            // tight/top gets (1-xBias)/(1-yBias) of that change.
            //
            // If a bias is missing it splits it down the middle.
            function zoomRange(g: Dygraph, zoomInPercentage: number, xBias: number, yBias: number) {
              const yAxes = g.yAxisRanges()
              const newYAxes = []
              for (let i = 0; i < yAxes.length; i += 1) {
                newYAxes[i] = adjustAxis(yAxes[i], zoomInPercentage, (yBias || 0.5))
              }

              return adjustAxis(g.xAxisRange(), zoomInPercentage, (xBias || 0.5))
            }

            if (event.altKey || event.shiftKey) {
              latestIsUserAction.current = true

              // http://dygraphs.com/gallery/interaction-api.js
              let normalDef
              // @ts-ignore
              if (typeof event.wheelDelta === "number" && !Number.isNaN(event.wheelDelta))
              // chrome
              {
                // @ts-ignore
                normalDef = event.wheelDelta / 40
              } else
              // firefox
              {
                normalDef = event.deltaY * -1.2
              }

              const normal = (event.detail) ? event.detail * -1 : normalDef
              const percentage = normal / 50

              const percentages = offsetToPercentage(dygraph, event.offsetX, event.offsetY)
              const xPct = percentages[0]
              const yPct = percentages[1]

              const [after, before] = zoomRange(dygraph, percentage, xPct, yPct)

              updateChartPanOrZoom({
                after, before,
                shouldNotExceedAvailableRange: true,
                callback: (updatedAfter: number, updatedBefore: number) => {
                  dygraph.updateOptions({
                    dateWindow: [updatedAfter, updatedBefore]
                  })
                }
              })

              event.preventDefault()
            }
          },

          click(event: MouseEvent, dygraph: Dygraph, context: any) {
            event.preventDefault()
            noop(dygraph, context)
          },

          touchstart(event: TouchEvent, dygraph: Dygraph, context: any) {
            isMouseDown.current = true
            latestIsUserAction.current = true

            // todo
            // state.pauseChart()

            Dygraph.defaultInteractionModel.touchstart(event, dygraph, context)

            // we overwrite the touch directions at the end, to overwrite
            // the internal default of dygraph
            // eslint-disable-next-line no-param-reassign
            context.touchDirections = { x: true, y: false }

            dygraphLastTouchMove.current = 0

            if (typeof event.touches[0].pageX === "number") {
              dygraphLastTouchPageX.current = event.touches[0].pageX
            } else {
              dygraphLastTouchPageX.current = 0
            }
          },
          touchmove(event: TouchEvent, dygraph: Dygraph, context: any) {
            latestIsUserAction.current = true
            Dygraph.defaultInteractionModel.touchmove(event, dygraph, context)

            dygraphLastTouchMove.current = Date.now()
          },

          touchend (event: TouchEvent, dygraph: Dygraph, context: any) {
            isMouseDown.current = false
            latestIsUserAction.current = true
            Dygraph.defaultInteractionModel.touchend(event, dygraph, context)

            // if it didn't move, it is a selection
            if (dygraphLastTouchMove.current === 0 && dygraphLastTouchPageX.current !== 0
              && chartElement.current // this is just for TS
            ) {
              // internal api of dygraph
              // @ts-ignore
              // eslint-disable-next-line no-underscore-dangle
              const dygraphPlotter = dygraph.plotter_
              const pct = (dygraphLastTouchPageX.current - (
                dygraphPlotter.area.x + chartElement.current.getBoundingClientRect().left
              )) / dygraphPlotter.area.w

              const { current } = propsRef
              const t = Math.round(current.viewAfter
                + (current.viewBefore - current.viewAfter) * pct
              )
              setHoveredX(t, true)
            }

            // if it was double tap within double click time, reset the charts
            const now = Date.now()
            if (typeof dygraphLastTouchEnd.current !== "undefined") {
              if (dygraphLastTouchMove.current === 0) {
                const dt = now - dygraphLastTouchEnd.current
                if (dt <= window.NETDATA.options.current.double_click_speed) {
                  // todo (reset)
                  // NETDATA.resetAllCharts(state)
                }
              }
            }

            // remember the timestamp of the last touch end
            dygraphLastTouchEnd.current = now
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
    dygraphInstance, globalChartUnderlay, hiddenLabelsElementId, isMouseDown, orderedColors,
    setGlobalChartUnderlay, setHoveredX, setMinMax, unitsCurrent, updateChartPanOrZoom,
    xAxisTimeString])

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

  // update chartOverlay
  useLayoutEffect(() => {
    if (dygraphInstance) {
      dygraphInstance.updateOptions({
        underlayCallback(canvas: CanvasRenderingContext2D, area: DygraphArea, g: Dygraph) {
          // the chart is about to be drawn
          // this function renders global highlighted time-frame

          if (globalChartUnderlay) {
            const { after, before } = globalChartUnderlay
            // todo limit that to view_after, view_before

            if (after < before) {
              const bottomLeft = g.toDomCoords(after, -20)
              const topRight = g.toDomCoords(before, +20)

              const left = bottomLeft[0]
              const right = topRight[0]

              // eslint-disable-next-line no-param-reassign
              canvas.fillStyle = window.NETDATA.themes.current.highlight
              canvas.fillRect(left, area.y, right - left, area.h)
            }
          }
        },
      })
    }
  }, [dygraphInstance, globalChartUnderlay])

  // update data of the chart
  useLayoutEffect(() => {
    if (dygraphInstance) {
      // todo support state.tmp.dygraph_force_zoom
      const forceDateWindow = [ viewAfter, viewBefore ]

      // in old dashboard, when chart needed to reset internal dateWindow state,
      // dateWindow was set to null, and new dygraph got the new dateWindow from results.
      // this caused small unsync between dateWindow of parent (master) and child charts
      // i also detected that forceDateWindow timestamps have slightly better performance (10%)
      // so if the chart needs to change local dateWindow, we'll always use timestamps
      const optionsDateWindow = isRemotelyControlled ? { dateWindow: forceDateWindow } : {}

      dygraphInstance.updateOptions({
        ...optionsDateWindow,
        file: chartData.result.data,
      })
    }
  }, [chartData.result.data, dygraphInstance, isRemotelyControlled, viewAfter, viewBefore])


  // set selection
  const currentSelectionMasterId = useSelector(selectGlobalSelectionMaster)
  useLayoutEffect(() => {
    if (dygraphInstance && currentSelectionMasterId !== chartUuid) {

      if (hoveredX === null) {
        // getSelection is 100 times faster that clearSelection
        if (dygraphInstance.getSelection() !== -1) {
          dygraphInstance.clearSelection()
        }
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
