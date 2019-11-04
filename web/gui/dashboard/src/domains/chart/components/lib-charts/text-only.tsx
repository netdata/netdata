import React from "react"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { EasyPieChartData } from "domains/chart/chart-types"

interface Props {
  attributes: Attributes
  chartData: EasyPieChartData
  chartElementClassName: string
  chartElementId: string
}
export const TextOnly = ({
  attributes,
  chartData,
  chartElementClassName,
  chartElementId,
}: Props) => {
  const {
    textOnlyDecimalPlaces = 1,
    textOnlyPrefix = "",
    textOnlySuffix = "",
  } = attributes

  // Round based on number of decimal places to show
  const precision = 10 ** textOnlyDecimalPlaces
  const value = Math.round(chartData.result[0] * precision) / precision

  const textContent = textOnlyPrefix + value + textOnlySuffix

  return (
    <div
      id={chartElementId}
      className={chartElementClassName}
    >
      {textContent}
    </div>
  )
}
