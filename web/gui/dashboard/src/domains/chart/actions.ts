import { createAction } from "redux-act"

import { ChartData } from "./chart-types"

export interface UpdateChartDataAction {
  chartData: ChartData
  id: string
}
export const updateChartDataAction = createAction<UpdateChartDataAction>("updateChartData")
