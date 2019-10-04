import React, { useRef, useEffect, useState } from "react"
import EasyPie from "easy-pie-chart"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { ChartDetails, EasyPieChartData } from "domains/chart/chart-types"
import { ChartLibraryName } from "domains/chart/utils/chartLibrariesSettings"
import {
  always, cond, identity, T, sortBy, compose, map,
} from "ramda"

type GetPercentFromValueMinMax = (arg: {
  value: number | undefined
  min: number | undefined
  max: number | undefined
  isMinOverride: boolean
  isMaxOverride: boolean
}) => number
const getPercentFromValueMinMax: GetPercentFromValueMinMax = ({
  value = 0, min = 0, max = 0,
  isMinOverride,
  isMaxOverride,
}) => {
  /* eslint-disable no-param-reassign */
  // todo refractor old logic to readable functions
  // if no easyPiechart-min-value attribute
  if (!isMinOverride && min > 0) {
    min = 0
  }
  if (!isMaxOverride && max < 0) {
    max = 0
  }

  let pcent

  if (min < 0 && max > 0) {
    // it is both positive and negative
    // zero at the top center of the chart
    max = (-min > max) ? -min : max
    pcent = Math.round((value * 100) / max)
  } else if (value >= 0 && min >= 0 && max >= 0) {
    // clockwise
    pcent = Math.round(((value - min) * 100) / (max - min))
    if (pcent === 0) {
      pcent = 0.1
    }
  } else {
    // counter clockwise
    pcent = Math.round(((value - max) * 100) / (max - min))
    if (pcent === 0) {
      pcent = -0.1
    }
  }
  /* eslint-enable no-param-reassign */
  return pcent
}

interface Props {
  attributes: Attributes
  chartData: EasyPieChartData
  chartDetails: ChartDetails
  chartLibrary: ChartLibraryName
  chartUuid: string
  colors: {
    [key: string]: string
  }
  chartWidth: number
  dimensionsVisibility: boolean[]
  isRemotelyControlled: boolean
  legendFormatValue: ((v: number | string) => number | string)
  onUpdateChartPanAndZoom: (arg: {
    after: number, before: number,
    callback: (after: number, before: number) => void,
    masterID: string,
    shouldNotExceedAvailableRange: boolean,
  }) => void
  orderedColors: string[]

  hoveredRow: number
  hoveredX: number | null
  setGlobalChartUnderlay: (arg: { after: number, before: number, masterID: string }) => void
  setHoveredX: (hoveredX: number | null, noMaster?: boolean) => void
  setMinMax: (minMax: [number, number]) => void
  unitsCurrent: string
  viewAfter: number
  viewBefore: number
}
export const EasyPieChart = ({
  attributes,
  chartData,
  chartDetails,
  chartWidth,
  hoveredRow,
  legendFormatValue,
  orderedColors,
  setMinMax,
  unitsCurrent,
}: Props) => {
  const chartElement = useRef<HTMLDivElement>(null)
  const [chartInstance, setChartInstance] = useState()

  const valueIndex = hoveredRow === -1
    ? 0
    : (chartData.result.length - 1 - hoveredRow) // because data for easy-pie-chart are flipped
  const value = chartData.result[valueIndex]

  const {
    // if this is set, then we're overriding commonMin
    easyPieChartMinValue: min = chartData.min, // todo replace with commonMin
    easyPieChartMaxValue: max = chartData.max, // todo replace with commonMax
  } = attributes

  // make sure the order is correct and that value is not outside those boundaries
  // (this check was present in old dashboard but perhaps it's not needed)
  const safeMinMax = compose(
    map((x: number) => +x),
    sortBy(identity),
    ([_min, _max]: [number, number]) => [Math.min(_min, value), Math.max(_max, value)],
  )([min, max])
  setMinMax(safeMinMax as [number, number])
  const pcent = getPercentFromValueMinMax({
    value,
    min: safeMinMax[0],
    max: safeMinMax[1],
    isMinOverride: attributes.easyPieChartMinValue !== undefined,
    isMaxOverride: attributes.easyPieChartMaxValue !== undefined,
  })


  useEffect(() => {
    if (chartElement.current && !chartInstance) {
      const stroke = cond([
        [(v) => v < 3, always(2)],
        [T, identity],
      ])(Math.floor(chartWidth / 22))

      const {
        easyPieChartTrackColor = window.NETDATA.themes.current.easypiechart_track,
        easyPieChartScaleColor = window.NETDATA.themes.current.easypiechart_scale,
        easyPieChartScaleLength = 5,
        easyPieChartLineCap = "round",
        easyPieChartLineWidth = stroke,
        easyPieChartTrackWidth,
        easyPieChartSize = chartWidth,
        easyPieChartRotate = 0,
        easyPieChartAnimate = { duration: 500, enabled: true },
        easyPieChartEasing,
      } = attributes

      const newChartInstance = new EasyPie(chartElement.current, {
        barColor: orderedColors[0],
        trackColor: easyPieChartTrackColor,
        scaleColor: easyPieChartScaleColor,
        scaleLength: easyPieChartScaleLength,
        lineCap: easyPieChartLineCap,
        lineWidth: easyPieChartLineWidth,
        trackWidth: easyPieChartTrackWidth,
        size: easyPieChartSize,
        rotate: easyPieChartRotate,
        animate: easyPieChartAnimate,
        easing: easyPieChartEasing,
      })
      setChartInstance(newChartInstance)
    }
  }, [attributes, chartData, chartInstance, chartWidth, orderedColors])

  // update with value
  useEffect(() => {
    if (chartInstance) {
      if (hoveredRow === -1 && !chartInstance.options.animate.enabled) {
        chartInstance.enableAnimation()
      } else if (hoveredRow !== -1 && chartInstance.options.animate.enabled) {
        chartInstance.disableAnimation()
      }
      chartInstance.update(pcent)
    }
  }, [chartInstance, hoveredRow, pcent])

  const valueFontSize = (chartWidth * 2) / 3 / 5
  const valuetop = Math.round((chartWidth - valueFontSize - (chartWidth / 40)) / 2)

  const titleFontSize = Math.round((valueFontSize * 1.6) / 3)
  const titletop = Math.round(valuetop - (titleFontSize * 2) - (chartWidth / 40))

  const unitFontSize = Math.round(titleFontSize * 0.9)
  const unitTop = Math.round(valuetop + (valueFontSize + unitFontSize) + (chartWidth / 40))
  // to update, just label innerText and pcent are changed

  return (
    <div ref={chartElement}>
      <span
        className="easyPieChartLabel"
        style={{
          fontSize: valueFontSize,
          top: valuetop,
        }}
      >
        {legendFormatValue(value)}
      </span>
      <span
        className="easyPieChartTitle"
        style={{
          fontSize: titleFontSize,
          top: titletop,
        }}
      >
        {attributes.title || chartDetails.title}
      </span>
      <span
        className="easyPieChartUnits"
        style={{
          fontSize: unitFontSize,
          top: unitTop,
        }}
      >
        {unitsCurrent}
      </span>

    </div>
  )
}
