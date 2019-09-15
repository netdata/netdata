import { createAction } from "redux-act"

import { createRequestAction } from "utils/createRequestAction"

import { storeKey } from "./constants"
import { ChartData, ChartDetails } from "./chart-types"

export interface UpdateChartDataAction {
  chartData: ChartData
  id: string
}
export const updateChartDataAction = createAction<UpdateChartDataAction>(`${storeKey}/updateChartData`)

export interface UpdateChartDetailsAction {
  chartDetails: ChartDetails
  id: string
}
export const updateChartDetailsAction = createAction<UpdateChartDetailsAction>(`${storeKey}/updateChartDetails`)

export interface FetchDataPayload {
  chart: string,
  format: string,
  points: number,
  group: string,
  gtime: number,
  options: string,
  after: number | null,
  before?: number | null,
  dimensions?: string,

  id: string,
}
export const fetchDataAction = createRequestAction<
  FetchDataPayload,
  { id: string, chartData: ChartData }
>(`${storeKey}/fetchDataAction`)
