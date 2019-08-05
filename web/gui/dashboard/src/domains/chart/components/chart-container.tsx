import React from "react"
import { useSelector } from "react-redux"

import { AppStateT } from "store/app-state"
import { Chart } from "./chart"
import { Attributes } from "../utils/transformDataAttributes"

import { selectChartData, selectChartDetails } from "../selectors"

export type Props = {
  attributes: Attributes
  chartUuid: string
}

export const ChartContainer = ({
  attributes,
  chartUuid,
}: Props) => {
  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: chartUuid }))
  const chartDetails = useSelector((state: AppStateT) => selectChartDetails(
    state, { id: chartUuid },
  ))
  if (!chartData || !chartDetails) {
    return <span>loading...</span>
  }
  return (
    <Chart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartUuid={chartUuid}
    />
  )
}
