import React, {
  useRef, useEffect, useState, useMemo,
} from "react"
import { Gauge } from "gaugeJS"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { ChartDetails, EasyPieChartData } from "domains/chart/chart-types"
import { ChartLibraryName } from "domains/chart/utils/chartLibrariesSettings"
import {
  identity, sortBy, map, pipe, always,
} from "ramda"

const isSetByUser = (x: undefined | number): x is number => (
  typeof x === "number"
)

interface Props {
  attributes: Attributes
  chartData: EasyPieChartData
  chartDetails: ChartDetails
  chartElementClassName: string
  chartElementId: string
  chartLibrary: ChartLibraryName
  chartUuid: string
  colors: {
    [key: string]: string
  }
  chartHeight: number
  chartWidth: number
  dimensionsVisibility: boolean[]
  isRemotelyControlled: boolean
  legendFormatValue: ((v: number | string | null) => number | string)
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
  showUndefined: boolean
  unitsCurrent: string
  viewAfter: number
  viewBefore: number
}
export const GaugeChart = ({
  attributes,
  chartData,
  chartDetails,
  chartElementClassName,
  chartElementId,
  chartUuid,
  chartHeight,
  chartWidth,
  hoveredRow,
  legendFormatValue,
  orderedColors,
  setMinMax,
  showUndefined,
  unitsCurrent,
}: Props) => {
  const chartCanvasElement = useRef<HTMLCanvasElement>(null)
  const [chartInstance, setChartInstance] = useState()

  const valueIndex = hoveredRow === -1
    ? 0
    : (chartData.result.length - 1 - hoveredRow) // because data for easy-pie-chart are flipped
  const value = chartData.result[valueIndex]

  const {
    // if this is set, then we're overriding commonMin
    gaugeMinValue: minAttribute,
    gaugeMaxValue: maxAttribute,
  } = attributes

  const min = isSetByUser(minAttribute) ? minAttribute : chartData.min
  const max = isSetByUser(maxAttribute) ? maxAttribute : chartData.max
  // we should use minAttribute if it's existing
  // old app was using commonMin

  // make sure the order is correct and that value is not outside those boundaries
  // (this check was present in old dashboard but perhaps it's not needed)
  const [safeMin, safeMax] = pipe(
    // if they are attributes, make sure they're converted to numbers
    map((x: number) => +x),
    // make sure it is zero based
    // but only if it has not been set by the user
    ([_min, _max]: number[]) => [
      (!isSetByUser(minAttribute) && _min > 0) ? 0 : _min,
      (!isSetByUser(maxAttribute) && _max < 0) ? 0 : _max,
    ],
    // make sure min <= max
    sortBy(identity),
    ([_min, _max]: number[]) => [Math.min(_min, value), Math.max(_max, value)],
  )([min, max])
  // calling outside "useEffect" intentionally,
  setMinMax([safeMin, safeMax])

  const pcent = pipe(
    always(((value - safeMin) * 100) / (safeMax - safeMin)),
    // bug fix for gauge.js 1.3.1
    // if the value is the absolute min or max, the chart is broken
    (_pcent: number) => Math.max(0.001, _pcent),
    (_pcent: number) => Math.min(99.999, _pcent),
  )()

  useEffect(() => {
    if (chartCanvasElement.current && !chartInstance) {
      const {
        gaugePointerColor = window.NETDATA.themes.current.gauge_pointer,
        gaugeStrokeColor = window.NETDATA.themes.current.gauge_stroke,
        gaugeStartColor = orderedColors[0],
        gaugeStopColor,
        gaugeGenerateGradient = false,
      } = attributes

      const options = {
        lines: 12, // The number of lines to draw
        angle: 0.14, // The span of the gauge arc
        lineWidth: 0.57, // The line thickness
        radiusScale: 1.0, // Relative radius
        pointer: {
          length: 0.85, // 0.9 The radius of the inner circle
          strokeWidth: 0.045, // The rotation offset
          color: gaugePointerColor, // Fill color
        },

        // If false, the max value of the gauge will be updated if value surpass max
        // If true, the min value of the gauge will be fixed unless you set it manually
        limitMax: true,
        limitMin: true,
        colorStart: gaugeStartColor,
        colorStop: gaugeStopColor,
        strokeColor: gaugeStrokeColor,
        generateGradient: (gaugeGenerateGradient === true), // gmosx:
        gradientType: 0,
        highDpiSupport: true, // High resolution support
      }

      const newChartInstance = new Gauge(chartCanvasElement.current).setOptions(options)

      // we will always feed a percentage (copied from old dashboard)
      newChartInstance.minValue = 0
      newChartInstance.maxValue = 100

      setChartInstance(newChartInstance)
    }
  }, [attributes, chartData, chartInstance, chartWidth, orderedColors])

  // update with value
  useEffect(() => {
    if (chartInstance) {
      // gauge animation
      const shouldUseAnimation = hoveredRow === -1 && !showUndefined
      // animation doesn't work in newest, 1.3.7 version!
      const speed = shouldUseAnimation ? 32 : 1000000000
      chartInstance.animationSpeed = speed
      setTimeout(() => {
        chartInstance.set(showUndefined ? 0 : pcent)
      }, 0)
    }
  }, [chartInstance, hoveredRow, pcent, showUndefined])

  // backwards-compatibility - in old dashboard the initial height was calculated based
  // on height of the component without gauge-chart rendered inside.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const chartHeightInitial = useMemo(() => chartHeight, [])

  const valueFontSize = Math.floor(chartHeightInitial / 5)
  const valueTop = Math.round((chartHeightInitial - valueFontSize) / 3.2)

  const titleFontSize = Math.round(valueFontSize / 2.1)
  const titleTop = 0

  const unitFontSize = Math.round(titleFontSize * 0.9)

  const minMaxFontSize = Math.round(valueFontSize * 0.75)
  return (
    <div
      id={chartElementId}
      className={chartElementClassName}
    >
      <canvas
        ref={chartCanvasElement}
        className="gaugeChart"
        id={`gauge-${chartUuid}-canvas`}
        width={chartWidth}
        height={chartHeightInitial}
      />
      <span
        className="gaugeChartLabel"
        style={{
          fontSize: valueFontSize,
          top: valueTop,
        }}
      >
        {legendFormatValue(showUndefined ? null : value)}
      </span>
      <span
        className="gaugeChartTitle"
        style={{
          fontSize: titleFontSize,
          top: titleTop,
        }}
      >
        {attributes.title || chartDetails.title}
      </span>
      <span
        className="gaugeChartUnits"
        style={{
          fontSize: unitFontSize,
        }}
      >
        {unitsCurrent}
      </span>
      <span className="gaugeChartMin" style={{ fontSize: minMaxFontSize }}>
        {legendFormatValue(showUndefined ? null : safeMin)}
      </span>
      <span className="gaugeChartMax" style={{ fontSize: minMaxFontSize }}>
        {legendFormatValue(showUndefined ? null : safeMax)}
      </span>
    </div>
  )
}
