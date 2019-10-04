import { mapObjIndexed } from "ramda"

import { ChartLibraryName } from "./chartLibrariesSettings"

type OutputValue = string | boolean | number | null | undefined | any[]
// almost the same as in old dashboard to ensure readers that it works the same way
const getDataAttribute = (element: Element, key: string, defaultValue?: OutputValue) => {
  const dataKey = `data-${key}`
  if (element.hasAttribute(dataKey)) {
    // we know it's not null because of hasAttribute()
    const data = element.getAttribute(dataKey) as string

    if (data === "true") {
      return true
    }
    if (data === "false") {
      return false
    }
    if (data === "null") {
      return null
    }

    // Only convert to a number if it doesn't change the string
    if (data === `${+data}`) {
      return +data
    }

    if (/^(?:\{[\w\W]*\}|\[[\w\W]*\])$/.test(data)) {
      return JSON.parse(data)
    }

    return data
  }
  // if no default is passed, then it's undefined and can be replaced with default value later
  // it is recommended to do it in props destructuring assignment phase, ie.:
  // const Chart = ({ dygraphPointsize = 1 }) => ....
  return defaultValue
}

const getDataAttributeBoolean = (element: Element, key: string, defaultValue?: boolean) => {
  const value = getDataAttribute(element, key, defaultValue)

  if (value === true || value === false) { // gmosx: Love this :)
    return value
  }

  if (typeof (value) === "string") {
    if (value === "yes" || value === "on") {
      return true
    }

    if (value === "" || value === "no" || value === "off" || value === "null") {
      return false
    }

    return defaultValue
  }

  if (typeof (value) === "number") {
    return value !== 0
  }

  return defaultValue
}

interface BaseAttributeConfig {
  key: string
  defaultValue?: OutputValue
}
interface BooleanAttributeConfig extends BaseAttributeConfig {
  type: "boolean"
  defaultValue?: boolean
}
type AttributeConfig = BaseAttributeConfig | BooleanAttributeConfig

export interface Attributes {
  id: string
  host: string
  title: string
  chartLibrary: ChartLibraryName
  width: number | string | null
  height: number | string | null
  after: number
  before: number
  legend: boolean
  units?: string
  unitsCommon?: string
  unitsDesired?: string
  colors?: string
  commonColors?: string
  decimalDigits?: number
  dimensions?: string

  appendOptions?: string | undefined
  gtime?: number
  method?: string
  overrideOptions?: string
  pixelsPerPoint?: number
  points?: number

  dygraphType?: string
  dygraphValueRange?: any[]
  dygraphTheme?: string
  dygraphSmooth?: boolean
  dygraphColors?: string[]
  dygraphRightGap?: number
  dygraphShowRangeSelector?: boolean
  dygraphShowRoller?: boolean
  dygraphTitle?: string
  dygraphTitleHeight?: number
  dygraphLegend?: "always" | "follow" | "onmouseover" | "never"
  dygraphLabelsDiv?: string
  dygraphLabelsSeparateLine?: boolean
  dygraphIncludeZero?: boolean
  dygraphShowZeroValues?: boolean
  dygraphShowLabelsOnHighLight?: boolean
  dygraphHideOverlayOnMouseOut?: boolean
  dygraphXRangePad?: number
  dygraphYRangePad?: number
  dygraphYLabelWidth?: number
  dygraphStrokeWidth?: number
  dygraphStrokePattern?: number[]
  dygraphDrawPoints?: boolean
  dygraphDrawGapEdgePoints?: boolean
  dygraphConnectSeparatedPoints?: boolean
  dygraphPointSize?: number
  dygraphStepPlot?: boolean
  dygraphStrokeBorderColor?: string
  dygraphStrokeBorderWidth?: number
  dygraphFillGraph?: boolean
  dygraphFillAlpha?: number
  dygraphStackedGraph?: boolean
  dygraphStackedGraphNanFill?: string
  dygraphAxisLabelFontSize?: number
  dygraphAxisLineColor?: string
  dygraphAxisLineWidth?: number
  dygraphDrawGrid?: boolean
  dygraphGridLinePattern?: number[]
  dygraphGridLineWidth?: number
  dygraphGridLineColor?: string
  dygraphMaxNumberWidth?: number
  dygraphSigFigs?: number
  dygraphDigitsAfterDecimal?: number
  dygraphHighlighCircleSize?: number
  dygraphHighlightSeriesOpts?: {[options: string]: number}
  dygraphHighlightSeriesBackgroundAlpha?: number
  dygraphXPixelsPerLabel?: number
  dygraphXAxisLabelWidth?: number
  dygraphDrawXAxis?: boolean
  dygraphYPixelsPerLabel?: number
  dygraphYAxisLabelWidth?: number
  dygraphDrawYAxis?: boolean
  dygraphDrawAxis?: boolean

  easyPieChartMinValue?: number
  easyPieChartMaxValue?: number
  easyPieChartBarColor?: string
  easyPieChartTrackColor?: string
  easyPieChartScaleColor?: string,
  easyPieChartScaleLength?: number,
  easyPieChartLineCap?: string,
  easyPieChartLineWidth?: string,
  easyPieChartTrackWidth?: string,
  easyPieChartSize?: string,
  easyPieChartRotate?: number,
  easyPieChartAnimate?: string,
  easyPieChartEasing?: string,
}

export type AttributePropKeys = keyof Attributes

type AttributesMap = {
  [key in AttributePropKeys]: AttributeConfig
}

// needs to be a getter so all window.NETDATA settings are set
const getAttributesMap = (): AttributesMap => ({
  // all properties that don't have `defaultValue` should be "| undefined" in Attributes interface
  // todo try to write above rule in TS
  id: { key: "netdata-react" },
  host: { key: "host", defaultValue: window.NETDATA.serverDefault },
  title: { key: "title" },
  chartLibrary: { key: "chart-library", defaultValue: window.NETDATA.chartDefaults.library },
  width: { key: "width", defaultValue: window.NETDATA.chartDefaults.width },
  height: { key: "height", defaultValue: window.NETDATA.chartDefaults.height },
  after: { key: "after", defaultValue: window.NETDATA.chartDefaults.after },
  before: { key: "before", defaultValue: window.NETDATA.chartDefaults.before },
  legend: { key: "legend", type: "boolean", defaultValue: true },
  units: { key: "units" },
  unitsCommon: { key: "common-units" },
  unitsDesired: { key: "desired-units" },
  colors: { key: "colors" },
  commonColors: { key: "common-colors" },
  decimalDigits: { key: "decimal-digits" },
  dimensions: { key: "dimensions" },

  appendOptions: { key: "append-options" },
  gtime: { key: "gtime" },
  method: { key: "method" },
  overrideOptions: { key: "override-options" },
  pixelsPerPoint: { key: "pixels-per-point" },
  points: { key: "points" },

  // let's not put the default values here, because they will also be needed by the main Agent page
  // and the Cloud App
  dygraphType: { key: "dygraph-type" },
  dygraphValueRange: { key: "dygraph-valuerange" },
  dygraphTheme: { key: "dygraph-theme" },
  dygraphSmooth: { key: "dygraph-smooth", type: "boolean" },
  dygraphColors: { key: "dygraph-colors" }, // not working in original dashboard
  dygraphRightGap: { key: "dygraph-rightgap" },
  dygraphShowRangeSelector: { key: "dygraph-showrangeselector", type: "boolean" },
  dygraphShowRoller: { key: "dygraph-showroller", type: "boolean" },
  dygraphTitle: { key: "dygraph-title" },
  dygraphTitleHeight: { key: "dygraph-titleheight" },
  dygraphLegend: { key: "dygraph-legend" },
  dygraphLabelsDiv: { key: "dygraph-labelsdiv" },
  dygraphLabelsSeparateLine: { key: "dygraph-labelsseparatelines", type: "boolean" },
  dygraphIncludeZero: { key: "dygraph-includezero", type: "boolean" },
  dygraphShowZeroValues: { key: "dygraph-labelsshowzerovalues", type: "boolean" },
  dygraphShowLabelsOnHighLight: { key: "dygraph-showlabelsonhighlight", type: "boolean" },
  dygraphHideOverlayOnMouseOut: { key: "dygraph-hideoverlayonmouseout", type: "boolean" },
  dygraphXRangePad: { key: "dygraph-xrangepad" },
  dygraphYRangePad: { key: "dygraph-yrangepad" },
  dygraphYLabelWidth: { key: "dygraph-ylabelwidth" },
  dygraphStrokeWidth: { key: "dygraph-strokewidth" },
  dygraphStrokePattern: { key: "dygraph-strokepattern" },
  dygraphDrawPoints: { key: "dygraph-drawpoints", type: "boolean" },
  dygraphDrawGapEdgePoints: { key: "dygraph-drawgapedgepoints", type: "boolean" },
  dygraphConnectSeparatedPoints: { key: "dygraph-connectseparatedpoints", type: "boolean" },
  dygraphPointSize: { key: "dygraph-pointsize" },
  dygraphStepPlot: { key: "dygraph-stepplot", type: "boolean" },
  dygraphStrokeBorderColor: { key: "dygraph-strokebordercolor" },
  dygraphStrokeBorderWidth: { key: "dygraph-strokeborderwidth" },
  // it was not boolean in the old app, but that was most likely a bug
  dygraphFillGraph: { key: "dygraph-fillgraph", type: "boolean" },
  dygraphFillAlpha: { key: "dygraph-fillalpha" },
  // also originally not set as boolean
  dygraphStackedGraph: { key: "dygraph-stackedgraph", type: "boolean" },
  dygraphStackedGraphNanFill: { key: "dygraph-stackedgraphnanfill" },
  dygraphAxisLabelFontSize: { key: "dygraph-axislabelfontsize" },
  dygraphAxisLineColor: { key: "dygraph-axislinecolor" },
  dygraphAxisLineWidth: { key: "dygraph-axislinewidth" },
  dygraphDrawGrid: { key: "dygraph-drawgrid", type: "boolean" },
  dygraphGridLinePattern: { key: "dygraph-gridlinepattern" },
  dygraphGridLineWidth: { key: "dygraph-gridlinewidth" },
  dygraphGridLineColor: { key: "dygraph-gridlinecolor" },
  dygraphMaxNumberWidth: { key: "dygraph-maxnumberwidth" },
  dygraphSigFigs: { key: "dygraph-sigfigs" },
  dygraphDigitsAfterDecimal: { key: "dygraph-digitsafterdecimal" },
  // dygraphValueFormatter: { key: "dygraph-valueformatter" },
  dygraphHighlighCircleSize: { key: "dygraph-highlightcirclesize" },
  dygraphHighlightSeriesOpts: { key: "dygraph-highlightseriesopts" },
  dygraphHighlightSeriesBackgroundAlpha: { key: "dygraph-highlightseriesbackgroundalpha" },
  // dygraphPointClickCallback: { key: "dygraph-pointclickcallback" },
  dygraphXPixelsPerLabel: { key: "dygraph-xpixelsperlabel" },
  dygraphXAxisLabelWidth: { key: "dygraph-xaxislabelwidth" },
  dygraphDrawXAxis: { key: "dygraph-drawxaxis", type: "boolean" },
  dygraphYPixelsPerLabel: { key: "dygraph-ypixelsperlabel" },
  dygraphYAxisLabelWidth: { key: "dygraph-yaxislabelwidth" },
  dygraphDrawYAxis: { key: "dygraph-drawyaxis", type: "boolean" },
  dygraphDrawAxis: { key: "dygraph-drawaxis", type: "boolean" },

  easyPieChartMinValue: { key: "easypiechart-min-value" },
  easyPieChartMaxValue: { key: "easypiechart-max-value" },
  easyPieChartBarColor: { key: "easypiechart-barcolor" },
  easyPieChartTrackColor: { key: "easypiechart-trackcolor" },
  easyPieChartScaleColor: { key: "easypiechart-scalecolor" },
  easyPieChartScaleLength: { key: "easypiechart-scalelength" },
  easyPieChartLineCap: { key: "easypiechart-linecap" },
  easyPieChartLineWidth: { key: "easypiechart-linewidth" },
  easyPieChartTrackWidth: { key: "easypiechart-trackwidth" },
  easyPieChartSize: { key: "easypiechart-size" },
  easyPieChartRotate: { key: "easypiechart-rotate" },
  easyPieChartAnimate: { key: "easypiechart-animate" },
  easyPieChartEasing: { key: "easypiechart-easing" },
})

export const getAttributes = (node: Element): Attributes => mapObjIndexed(
  (attribute: AttributeConfig) => (
    (attribute as BooleanAttributeConfig).type === "boolean"
      ? getDataAttributeBoolean(
        node,
        attribute.key,
          attribute.defaultValue as BooleanAttributeConfig["defaultValue"],
      ) : getDataAttribute(node, attribute.key, attribute.defaultValue)
  ),
  getAttributesMap(),
) as Attributes // need to override because of broken Ramda typings
