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
  units: string
  colors: string | undefined
  commonColors: string | undefined

  dygraphValueRange: any[]
  dygraphTheme: string
  dygraphColors: string[] | undefined
  dygraphRightGap: number
  dygraphShowRangeSelector: boolean
  dygraphShowRoller: boolean
  dygraphTitle: string | undefined
  dygraphTitleHeight: number
  dygraphLegend: "always" | "follow" | "onmouseover" | "never" | undefined
  dygraphLabelsDiv: string | undefined
  dygraphLabelsSeparateLine: boolean
  dygraphShowZeroValues: boolean
  dygraphShowLabelsOnHighLight: boolean
  dygraphHideOverlayOnMouseOut: boolean
  dygraphXRangePad: number
  dygraphYRangePad: number
  dygraphYLabelWidth: number
  dygraphStrokeWidth: number | undefined
  dygraphStrokePattern: number[] | undefined
  dygraphDrawPoints: boolean
  dygraphDrawGapEdgePoints: boolean
  dygraphConnectSeparatedPoints: boolean | undefined
  dygraphPointSize: number | undefined
  dygraphStepPlot: boolean | undefined
  dygraphStrokeBorderColor: string | undefined
  dygraphStrokeBorderWidth: number | undefined
  dygraphFillGraph: boolean | undefined
  dygraphFillAlpha: number | undefined
  dygraphStackedGraph: boolean | undefined
  dygraphStackedGraphNanFill: string | undefined
  dygraphAxisLabelFontSize: number | undefined
  dygraphAxisLineColor: string | undefined
  dygraphAxisLineWidth: number | undefined
  dygraphDrawGrid: boolean | undefined
  dygraphGridLinePattern: number[] | undefined
  dygraphGridLineWidth: number | undefined
  dygraphGridLineColor: string | undefined
  dygraphMaxNumberWidth: number | undefined
  dygraphSigFigs: number | undefined
  dygraphDigitsAfterDecimal: number | undefined
  dygraphHighlighCircleSize: number | undefined
  dygraphHighlightSeriesOpts: {[options: string]: number} | undefined
  dygraphHighlightSeriesBackgroundAlpha: number | undefined
  dygraphXPixelsPerLabel: number | undefined
  dygraphXAxisLabelWidth: number | undefined
  dygraphDygraphDrawXAxis: boolean | undefined
  dygraphYPixelsPerLabel: number | undefined
  dygraphYAxisLabelWidth: number | undefined
  dygraphDrawYAxis: boolean | undefined
  dygraphDrawAxis: boolean | undefined
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
  title: { key: "title", defaultValue: null },
  chartLibrary: { key: "chart-library", defaultValue: window.NETDATA.chartDefaults.library },
  width: { key: "width", defaultValue: window.NETDATA.chartDefaults.width },
  height: { key: "height", defaultValue: window.NETDATA.chartDefaults.height },
  after: { key: "after", defaultValue: window.NETDATA.chartDefaults.after },
  before: { key: "after", defaultValue: window.NETDATA.chartDefaults.before },
  legend: { key: "legend", type: "boolean", defaultValue: true },
  units: { key: "units" },
  colors: { key: "colors" },
  commonColors: { key: "common-colors" },

  // dygraph
  // let's not put the default values here, because they will also be needed by the main Agent page
  // and the Cloud App
  dygraphValueRange: { key: "dygraph-valuerange", defaultValue: [null, null] },
  dygraphTheme: { key: "dygraph-theme", defaultValue: "default" },
  dygraphColors: { key: "dygraph-colors" }, // not working in original dashboard
  dygraphRightGap: { key: "dygraph-rightgap", defaultValue: 5 },
  dygraphShowRangeSelector: {
    key: "dygraph-showrangeselector", type: "boolean", defaultValue: false,
  },
  dygraphShowRoller: { key: "dygraph-showroller", type: "boolean", defaultValue: false },
  dygraphTitle: { key: "dygraph-title" },
  dygraphTitleHeight: { key: "dygraph-titleheight", defaultValue: 19 },
  dygraphLegend: { key: "dygraph-legend" },
  dygraphLabelsDiv: { key: "dygraph-labelsdiv" },
  dygraphLabelsSeparateLine: {
    key: "dygraph-labelsseparatelines", type: "boolean", defaultValue: true,
  },
  dygraphShowZeroValues: {
    key: "dygraph-labelsshowzerovalues", type: "boolean", defaultValue: true,
  },
  dygraphShowLabelsOnHighLight: {
    key: "dygraph-showlabelsonhighlight", type: "boolean", defaultValue: true,
  },
  dygraphHideOverlayOnMouseOut: {
    key: "dygraph-hideoverlayonmouseout", type: "boolean", defaultValue: true,
  },
  dygraphXRangePad: { key: "dygraph-xrangepad", defaultValue: 0 },
  dygraphYRangePad: { key: "dygraph-yrangepad", defaultValue: 0 },
  dygraphYLabelWidth: { key: "dygraph-ylabelwidth", defaultValue: 12 },
  dygraphStrokeWidth: { key: "dygraph-strokewidth" },
  dygraphStrokePattern: { key: "dygraph-strokepattern" },
  dygraphDrawPoints: {
    key: "dygraph-drawpoints", type: "boolean", defaultValue: false,
  },
  dygraphDrawGapEdgePoints: {
    key: "dygraph-drawgapedgepoints", type: "boolean", defaultValue: true,
  },


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
  dygraphDygraphDrawXAxis: { key: "dygraph-drawxaxis", type: "boolean" },
  dygraphYPixelsPerLabel: { key: "dygraph-ypixelsperlabel" },
  dygraphYAxisLabelWidth: { key: "dygraph-yaxislabelwidth" },
  dygraphDrawYAxis: { key: "dygraph-drawyaxis", type: "boolean" },
  dygraphDrawAxis: { key: "dygraph-drawaxis", type: "boolean" },
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
