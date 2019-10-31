import "peity"
import React, {
  useRef, useEffect, useState, useLayoutEffect,
} from "react"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { ChartDetails, EasyPieChartData } from "domains/chart/chart-types"
import { colorLuminance } from "domains/chart/utils/color-luminance"

// import "../../utils/peity-loader"


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
export const PeityChart = ({
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
  const peityOptions = useRef<{
    stroke: string,
    fill: string,
    strokeWidth: number,
    width: number,
    height: number,
  }>()


  // create chart
  useLayoutEffect(() => {
    if (chartElement.current && !$chartElement) {
      const $element = $(chartElement.current)

      const { width, height } = chartContainerElement.getBoundingClientRect()

      const strokeWidth2 = 1 // todo NETDATA.dataAttribute(state.element, 'peity-strokewidth', 1),
      const {
        peityStrokeWidth = 1,
      } = attributes
      console.log("peityStrokeWidth", peityStrokeWidth) // eslint-disable-line no-console
      console.log("typeof peityStrokeWidth", typeof peityStrokeWidth) // eslint-disable-line no-console
      const peityInitOptions = {
        stroke: window.NETDATA.themes.current.foreground,
        strokeWidth: peityStrokeWidth,
        width: Math.floor(width),
        height: Math.floor(height),
        fill: window.NETDATA.themes.current.foreground,
      }

      set$chartElement(() => $element)
      peityOptions.current = peityInitOptions
    }
  }, [attributes, $chartElement, chartContainerElement])

  // update chart
  useLayoutEffect(() => {
    if ($chartElement && peityOptions.current) {
      // if stroke is different than customColors, then if chart_type is 'line', then apply fill
      const getFillOverride = () => (
        chartDetails.chart_type === "line"
          ? window.NETDATA.themes.current.background
          : colorLuminance(orderedColors[0], window.NETDATA.chartDefaults.fill_luminance)
      )
      const updatedOptions = {
        ...peityOptions.current,
        stroke: orderedColors[0],
        // optimizatino from old dashboard, perhaps could be transformed to useMemo()
        fill: (orderedColors[0] === peityOptions.current.stroke)
          ? peityOptions.current.fill
          : getFillOverride(),
      }
      $chartElement.peity("line", updatedOptions)
      peityOptions.current = updatedOptions
    }
  }, [$chartElement, chartData, chartDetails, orderedColors])

  return (
    <div
      ref={chartElement}
      id={chartElementId}
      className={chartElementClassName}
    >
      {chartData.result}
    </div>
  )
}
