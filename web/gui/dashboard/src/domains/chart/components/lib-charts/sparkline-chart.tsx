import "jquery-sparkline"
import React, {
  useRef, useEffect, useState,
} from "react"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { ChartDetails, EasyPieChartData } from "domains/chart/chart-types"
import { colorLuminance } from "domains/chart/utils/color-luminance"

interface Props {
  attributes: Attributes
  chartContainerElement: HTMLElement
  chartData: EasyPieChartData
  chartDetails: ChartDetails
  chartElementClassName: string
  chartElementId: string
  dimensionsVisibility: boolean[]
  isRemotelyControlled: boolean
  orderedColors: string[]
  unitsCurrent: string
}
export const SparklineChart = ({
  attributes,
  chartContainerElement,
  chartData,
  chartDetails,
  chartElementClassName,
  chartElementId,
  orderedColors,
  unitsCurrent,
}: Props) => {
  const chartElement = useRef<HTMLDivElement>(null)

  // update width, height automatically each time
  const [$chartElement, set$chartElement] = useState()
  const sparklineOptions = useRef<{[key: string]: any}>()

  // create chart
  useEffect(() => {
    if (chartElement.current && !$chartElement) {
      const $element = $(chartElement.current)
      const { width, height } = chartContainerElement.getBoundingClientRect()
      const {
        sparklineLineColor = orderedColors[0],
      } = attributes
      const defaultFillColor = chartDetails.chart_type === "line"
        ? window.NETDATA.themes.current.background
        : colorLuminance(sparklineLineColor, window.NETDATA.chartDefaults.fill_luminance)
      const {
        sparklineType = "line",
        sparklineFillColor = defaultFillColor,
        sparklineDisableInteraction = false,
        sparklineDisableTooltips = false,
        sparklineDisableHighlight = false,
        sparklineHighlightLighten = 1.4,
        sparklineTooltipSuffix = ` ${unitsCurrent}`,
        sparklineNumberFormatter = (n: number) => n.toFixed(2),
      } = attributes
      const chartTitle = attributes.title || chartDetails.title

      const emptyStringIfDisable = (x: string | undefined) => (x === "disable" ? "" : x)
      const sparklineInitOptions = {
        type: sparklineType,
        lineColor: sparklineLineColor,
        fillColor: sparklineFillColor,
        chartRangeMin: attributes.sparklineChartRangeMin,
        chartRangeMax: attributes.sparklineChartRangeMax,
        composite: attributes.sparklineComposite,
        enableTagOptions: attributes.sparklineEnableTagOptions,
        tagOptionPrefix: attributes.sparklineTagOptionPrefix,
        tagValuesAttribute: attributes.sparklineTagValuesAttribute,

        disableHiddenCheck: attributes.sparklineDisableHiddenCheck,
        defaultPixelsPerValue: attributes.sparklineDefaultPixelsPerValue,
        spotColor: emptyStringIfDisable(attributes.sparklineSpotColor),
        minSpotColor: emptyStringIfDisable(attributes.sparklineMinSpotColor),
        maxSpotColor: emptyStringIfDisable(attributes.sparklineMaxSpotColor),
        spotRadius: attributes.sparklineSpotRadius,
        valueSpots: attributes.sparklineValueSpots,
        highlightSpotColor: attributes.sparklineHighlightSpotColor,
        highlightLineColor: attributes.sparklineHighlightLineColor,
        lineWidth: attributes.sparklineLineWidth,
        normalRangeMin: attributes.sparklineNormalRangeMin,
        normalRangeMax: attributes.sparklineNormalRangeMax,
        drawNormalOnTop: attributes.sparklineDrawNormalOnTop,
        xvalues: attributes.sparklineXvalues,
        chartRangeClip: attributes.sparklineChartRangeClip,
        chartRangeMinX: attributes.sparklineChartRangeMinX,
        chartRangeMaxX: attributes.sparklineChartRangeMaxX,
        disableInteraction: sparklineDisableInteraction,
        disableTooltips: sparklineDisableTooltips,
        disableHighlight: sparklineDisableHighlight,
        highlightLighten: sparklineHighlightLighten,
        highlightColor: attributes.sparklineHighlightColor,
        tooltipContainer: attributes.sparklineTooltipContainer,
        tooltipClassname: attributes.sparklineTooltipClassname,
        tooltipChartTitle: chartTitle,
        tooltipFormat: attributes.sparklineTooltipFormat,
        tooltipPrefix: attributes.sparklineTooltipPrefix,
        tooltipSuffix: sparklineTooltipSuffix,
        tooltipSkipNull: attributes.sparklineTooltipSkipNull,
        tooltipValueLookups: attributes.sparklineTooltipValueLookups,
        tooltipFormatFieldlist: attributes.sparklineTooltipFormatFieldlist,
        tooltipFormatFieldlistKey: attributes.sparklineTooltipFormatFieldlistKey,
        numberFormatter: sparklineNumberFormatter,
        numberDigitGroupSep: attributes.sparklineNumberDigitGroupSep,
        numberDecimalMark: attributes.sparklineNumberDecimalMark,
        numberDigitGroupCount: attributes.sparklineNumberDigitGroupCount,
        animatedZooms: attributes.sparklineAnimatedZooms,
        width: Math.floor(width),
        height: Math.floor(height),
      }

      set$chartElement(() => $element)
      sparklineOptions.current = sparklineInitOptions
      // @ts-ignore
      $element.sparkline(chartData.result, sparklineInitOptions)
    }
  }, [$chartElement, attributes, chartContainerElement, chartData.result, chartDetails,
    orderedColors, unitsCurrent])

  // update chart
  useEffect(() => {
    if ($chartElement) {
      const { width, height } = chartContainerElement.getBoundingClientRect()
      $chartElement.sparkline(chartData.result, {
        ...sparklineOptions.current,
        width: Math.floor(width),
        height: Math.floor(height),
      })
    }
  })

  return (
    <div ref={chartElement} id={chartElementId} className={chartElementClassName} />
  )
}
