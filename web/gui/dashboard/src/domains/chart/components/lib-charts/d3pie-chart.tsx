import "jquery-sparkline"
import React, {
  useRef, useEffect, useState,
} from "react"

import "../../utils/d3-loader"
import d3pie from "vendor/d3pie-0.2.1-netdata-3"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import {
  ChartDetails,
  D3pieChartData,
} from "domains/chart/chart-types"
import { seconds4human } from "domains/chart/utils/seconds4human"
import { useDateTime } from "utils/date-time"
import { tail } from "ramda"

const emptyContent = {
  label: "no data",
  value: 100,
  color: "#666666",
}

type GetDateRange = (arg: {
  chartData: D3pieChartData,
  index: number,
  localeDateString: (date: number | Date) => string,
  localeTimeString: (time: number | Date) => string,
}) => string
const getDateRange: GetDateRange = ({
  chartData, index,
  localeDateString, localeTimeString,
}) => {
  const dt = Math.round((chartData.before - chartData.after + 1) / chartData.points)
  const dtString = seconds4human(dt)

  const before = chartData.result.data[index].time
  const after = before - (dt * 1000)

  const d1 = localeDateString(after)
  const t1 = localeTimeString(after)
  const d2 = localeDateString(before)
  const t2 = localeTimeString(before)

  if (d1 === d2) {
    return `${d1} ${t1} to ${t2}, ${dtString}`
  }

  return `${d1} ${t1} to ${d2} ${t2}, ${dtString}`
}

interface Props {
  attributes: Attributes
  chartContainerElement: HTMLElement
  chartData: D3pieChartData
  chartDetails: ChartDetails
  chartElementClassName: string
  chartElementId: string
  dimensionsVisibility: boolean[]
  hoveredRow: number
  hoveredX: number | null
  isRemotelyControlled: boolean
  legendFormatValue: ((v: number | string | null) => number | string)
  orderedColors: string[]
  setMinMax: (minMax: [number, number]) => void
  showUndefined: boolean
  unitsCurrent: string
}
export const D3pieChart = ({
  attributes,
  chartContainerElement,
  chartData,
  chartDetails,
  chartElementClassName,
  chartElementId,
  hoveredRow,
  hoveredX,
  legendFormatValue,
  orderedColors,
  setMinMax,
  unitsCurrent,
}: Props) => {
  const chartElement = useRef<HTMLDivElement>(null)

  const [d3pieInstance, setD3pieInstance] = useState()
  const d3pieOptions = useRef<{[key: string]: any}>()

  const { localeDateString, localeTimeString } = useDateTime()

  // create chart
  useEffect(() => {
    if (chartElement.current && !d3pieInstance) {
      // d3pieSetContent
      // todo this should be set in chart.tsx, when creating hook
      setMinMax([chartData.min, chartData.max])
      // index is ROW! it's !== 0 only when selection is made
      const index = 0
      const content = tail(chartData.result.labels).map((label, i) => {
        const value = chartData.result.data[index][label]
        const color = orderedColors[i]
        return {
          label,
          value,
          color,
        }
      }).filter((x) => x.value !== null && x.value > 0)
      const safeContent = content.length > 0 ? content : emptyContent

      const defaultTitle = attributes.title || chartDetails.title
      const dateRange = getDateRange({
        chartData,
        index: 0,
        localeDateString,
        localeTimeString,
      })
      const {
        d3pieTitle = defaultTitle,
        d3pieSubtitle = unitsCurrent,
        d3pieFooter = dateRange,
        d3pieTitleColor = window.NETDATA.themes.current.d3pie.title,
        d3pieTitleFontsize = 12,
        d3pieTitleFontweight = "bold",
        d3pieTitleFont = "arial",
        d3PieSubtitleColor = window.NETDATA.themes.current.d3pie.subtitle,
        d3PieSubtitleFontsize = 10,
        d3PieSubtitleFontweight = "normal",
        d3PieSubtitleFont = "arial",
        d3PieFooterColor = window.NETDATA.themes.current.d3pie.footer,
        d3PieFooterFontsize = 9,
        d3PieFooterFontweight = "bold",
        d3PieFooterFont = "arial",
        d3PieFooterLocation = "bottom-center",

        d3PiePieinnerradius = "45%",
        d3PiePieouterradius = "80%",
        d3PieSortorder = "value-desc",
        d3PieSmallsegmentgroupingEnabled = false,
        d3PieSmallsegmentgroupingValue = 1,
        d3PieSmallsegmentgroupingValuetype = "percentage",
        d3PieSmallsegmentgroupingLabel = "other",
        d3PieSmallsegmentgroupingColor = window.NETDATA.themes.current.d3pie.other,

        d3PieLabelsOuterFormat = "label-value1",
        d3PieLabelsOuterHidewhenlessthanpercentage = null,
        d3PieLabelsOuterPiedistance = 15,
        d3PieLabelsInnerFormat = "percentage",
        d3PieLabelsInnerHidewhenlessthanpercentage = 2,

        d3PieLabelsMainLabelColor = window.NETDATA.themes.current.d3pie.mainlabel,
        d3PieLabelsMainLabelFont = "arial",
        d3PieLabelsMainLabelFontsize = 10,
        d3PieLabelsMainLabelFontweight = "normal",

        d3PieLabelsPercentageColor = window.NETDATA.themes.current.d3pie.percentage,
        d3PieLabelsPercentageFont = "arial",
        d3PieLabelsPercentageFontsize = 10,
        d3PieLabelsPercentageFontweight = "bold",

        d3PieLabelsValueColor = window.NETDATA.themes.current.d3pie.value,
        d3PieLabelsValueFont = "arial",
        d3PieLabelsValueFontsize = 10,
        d3PieLabelsValueFontweight = "bold",

        d3PieLabelsLinesEnabled = true,
        d3PieLabelsLinesStyle = "curved",
        d3PieLabelsLinesColor = "segment", // "segment" or a hex color

        d3PieLabelsTruncationEnabled = false,
        d3PieLabelsTruncationTruncatelength = 30,

        d3PieMiscColorsSegmentstroke = window.NETDATA.themes.current.d3pie.segment_stroke,
        d3PieMiscGradientEnabled = false,
        d3PieMiscColorsPercentage = 95,
        d3PieMiscGradientColor = window.NETDATA.themes.current.d3pie.gradient_color,

        d3PieCssprefix = null,
      } = attributes

      const { width, height } = chartContainerElement.getBoundingClientRect()

      const initialD3pieOptions = {
        header: {
          title: {
            text: d3pieTitle,
            color: d3pieTitleColor,
            fontSize: d3pieTitleFontsize,
            fontWeight: d3pieTitleFontweight,
            font: d3pieTitleFont,
          },
          subtitle: {
            text: d3pieSubtitle,
            color: d3PieSubtitleColor,
            fontSize: d3PieSubtitleFontsize,
            fontWeight: d3PieSubtitleFontweight,
            font: d3PieSubtitleFont,
          },
          titleSubtitlePadding: 1,
        },
        footer: {
          text: d3pieFooter,
          color: d3PieFooterColor,
          fontSize: d3PieFooterFontsize,
          fontWeight: d3PieFooterFontweight,
          font: d3PieFooterFont,
          location: d3PieFooterLocation,
        },
        size: {
          canvasHeight: Math.floor(height),
          canvasWidth: Math.floor(width),
          pieInnerRadius: d3PiePieinnerradius,
          pieOuterRadius: d3PiePieouterradius,
        },
        data: {
          // none, random, value-asc, value-desc, label-asc, label-desc
          sortOrder: d3PieSortorder,
          smallSegmentGrouping: {
            enabled: d3PieSmallsegmentgroupingEnabled,
            value: d3PieSmallsegmentgroupingValue,
            // percentage, value
            valueType: d3PieSmallsegmentgroupingValuetype,
            label: d3PieSmallsegmentgroupingLabel,
            color: d3PieSmallsegmentgroupingColor,
          },

          // REQUIRED! This is where you enter your pie data; it needs to be an array of objects
          // of this form: { label: "label", value: 1.5, color: "#000000" } - color is optional
          content: safeContent,
        },


        labels: {
          outer: {
            // label, value, percentage, label-value1, label-value2, label-percentage1,
            // label-percentage2
            format: d3PieLabelsOuterFormat,
            hideWhenLessThanPercentage: d3PieLabelsOuterHidewhenlessthanpercentage,
            pieDistance: d3PieLabelsOuterPiedistance,
          },
          inner: {
            // label, value, percentage, label-value1, label-value2, label-percentage1,
            // label-percentage2
            format: d3PieLabelsInnerFormat,
            hideWhenLessThanPercentage: d3PieLabelsInnerHidewhenlessthanpercentage,
          },
          mainLabel: {
            color: d3PieLabelsMainLabelColor, // or 'segment' for dynamic color
            font: d3PieLabelsMainLabelFont,
            fontSize: d3PieLabelsMainLabelFontsize,
            fontWeight: d3PieLabelsMainLabelFontweight,
          },
          percentage: {
            color: d3PieLabelsPercentageColor,
            font: d3PieLabelsPercentageFont,
            fontSize: d3PieLabelsPercentageFontsize,
            fontWeight: d3PieLabelsPercentageFontweight,
            decimalPlaces: 0,
          },
          value: {
            color: d3PieLabelsValueColor,
            font: d3PieLabelsValueFont,
            fontSize: d3PieLabelsValueFontsize,
            fontWeight: d3PieLabelsValueFontweight,
          },
          lines: {
            enabled: d3PieLabelsLinesEnabled,
            style: d3PieLabelsLinesStyle,
            color: d3PieLabelsLinesColor,
          },
          truncation: {
            enabled: d3PieLabelsTruncationEnabled,
            truncateLength: d3PieLabelsTruncationTruncatelength,
          },
          formatter(context: any) {
            if (context.part === "value") {
              return legendFormatValue(context.value)
            }
            if (context.part === "percentage") {
              return `${context.label}%`
            }

            return context.label
          },
        },
        effects: {
          load: {
            effect: "none", // none / default
            speed: 0, // commented in the d3pie code to speed it up
          },
          pullOutSegmentOnClick: {
            effect: "bounce", // none / linear / bounce / elastic / back
            speed: 400,
            size: 5,
          },
          highlightSegmentOnMouseover: true,
          highlightLuminosity: -0.2,
        },
        tooltips: {
          enabled: false,
          type: "placeholder", // caption|placeholder
          string: "",
          placeholderParser: null, // function
          styles: {
            fadeInSpeed: 250,
            backgroundColor: window.NETDATA.themes.current.d3pie.tooltip_bg,
            backgroundOpacity: 0.5,
            color: window.NETDATA.themes.current.d3pie.tooltip_fg,
            borderRadius: 2,
            font: "arial",
            fontSize: 12,
            padding: 4,
          },
        },
        misc: {
          colors: {
            background: "transparent", // transparent or color #
            // segments: state.chartColors(),
            segmentStroke: d3PieMiscColorsSegmentstroke,
          },
          gradient: {
            enabled: d3PieMiscGradientEnabled,
            percentage: d3PieMiscColorsPercentage,
            color: d3PieMiscGradientColor,
          },
          canvasPadding: {
            top: 5,
            right: 5,
            bottom: 5,
            left: 5,
          },
          pieCenterOffset: {
            x: 0,
            y: 0,
          },
          cssPrefix: d3PieCssprefix,
        },
        callbacks: {
          onload: null,
          onMouseoverSegment: null,
          onMouseoutSegment: null,
          onClickSegment: null,
        },
      }
      // eslint-disable-next-line new-cap
      const newD3pieInstance = new d3pie(chartElement.current, initialD3pieOptions)
      d3pieOptions.current = initialD3pieOptions
      setD3pieInstance(() => newD3pieInstance)
    }
  }, [attributes, chartContainerElement, chartData, chartDetails, d3pieInstance, legendFormatValue,
    localeDateString, localeTimeString, orderedColors, setMinMax, unitsCurrent])

  // update chart
  useEffect(() => {
    if (d3pieInstance && d3pieOptions.current) {
      const dateRange = getDateRange({
        chartData,
        index: 0,
        localeDateString,
        localeTimeString,
      })
      const {
        d3pieSubtitle = unitsCurrent,
        d3pieFooter = dateRange,
      } = attributes


      const isHoveredButNoData = !!hoveredX && (hoveredRow === -1)
      const slot = chartData.result.data.length - hoveredRow - 1

      const index = (slot < 0 || slot >= chartData.result.data.length)
        ? 0
        : slot

      const content = tail(chartData.result.labels).map((label, i) => {
        const value = chartData.result.data[index][label]
        const color = orderedColors[i]
        return {
          label,
          value,
          color,
        }
      }).filter((x) => x.value !== null && x.value > 0)
      const safeContent = (content.length > 0 && !isHoveredButNoData)
        ? content
        : [emptyContent]

      d3pieInstance.options.header.subtitle.text = d3pieSubtitle
      d3pieInstance.options.footer.text = d3pieFooter

      d3pieInstance.options.data.content = safeContent
      d3pieInstance.destroy()
      d3pieInstance.recreate()
    }
  }, [attributes, chartData, d3pieInstance, hoveredRow, hoveredX, localeDateString,
    localeTimeString, orderedColors, unitsCurrent])

  return (
    <div ref={chartElement} id={chartElementId} className={chartElementClassName} />
  )
}
