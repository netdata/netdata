import React from "react"
import { useSelector } from "react-redux"

import { AppStateT } from "store/app-state"

import { selectChartData } from "../domains/chart/selectors"

export type Props = {
  // settings from DOM node
  id: string
  host?: string
  title?: string
  chartLibrary?: string
  width?: string
  height?: string
  after?: string
  dygraphValueRange?: string

  uniqueId: string
}

export const ChartContainer = ({
  uniqueId,
}: Props) => {
  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: uniqueId }))
  console.log("chartData", chartData) // eslint-disable-line no-console
  return (
    <div>Chart Container</div>
  )
}
