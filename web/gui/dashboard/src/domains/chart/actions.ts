import { createAction } from "redux-act"

import { ChartData, ChartDetails } from "./chart-types"

export interface UpdateChartDataAction {
  chartData: ChartData
  id: string
}
export const updateChartDataAction = createAction<UpdateChartDataAction>("updateChartData")

export interface UpdateChartDetailsAction {
  chartDetails: ChartDetails
  id: string
}
export const updateChartDetailsAction = createAction<UpdateChartDetailsAction>("updateChartDetails")
