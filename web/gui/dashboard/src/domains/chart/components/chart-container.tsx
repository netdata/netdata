import React from "react"
import { useSelector } from "react-redux"

import { AppStateT } from "store/app-state"
import { Chart } from "./chart"
import { Attributes } from "../utils/transformDataAttributes"

import { selectChartData } from "../selectors"

export type Props = {
  attributes: Attributes
  uniqueId: string
}

export const ChartContainer = ({
  attributes,
  uniqueId,
}: Props) => {
  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: uniqueId }))
  if (!chartData) {
    return <span>loading...</span>
  }
  return (
    <Chart
      chartData={chartData}
      attributes={attributes}
    />
  )
}
