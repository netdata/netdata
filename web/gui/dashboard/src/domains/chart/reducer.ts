import { createReducer } from "redux-act"

import { updateChartDataAction } from "./actions"
import { ChartState } from "./chart-types"

export type StateT = {
  [chartId: string]: ChartState
}

const initialState = {
}
export const initialSingleState = {
  chartData: null,
}

export const chartReducer = createReducer<StateT>(
  {},
  initialState,
)

const getSubstate = (state: StateT, id: string) => state[id] || initialSingleState

chartReducer.on(updateChartDataAction, (state, { id, chartData }) => ({
  ...state,
  [id]: {
    ...getSubstate(state, id),
    chartData,
  },
}))

export const chartKey = "chart"
