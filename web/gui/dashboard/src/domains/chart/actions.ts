import { createAction } from "redux-act"

import { createRequestAction } from "utils/createRequestAction"
import { Attributes } from "domains/chart/utils/transformDataAttributes"

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
  attributes: Attributes,
  id: string,
  uuid: string,
}
export const fetchDataAction = createRequestAction<
  FetchDataPayload,
  { id: string, chartData: ChartData }
>(`${storeKey}/fetchDataAction`)
