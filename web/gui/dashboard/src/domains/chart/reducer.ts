import { createReducer } from "redux-act"

import { fetchDataAction, fetchChartAction, setResizeHeightAction } from "./actions"
import { ChartState } from "./chart-types"

export type StateT = {
  [chartId: string]: ChartState
}

export const initialState = {
}
export const initialSingleState = {
  chartData: null,
  chartDetails: null,
  fetchDataParams: {
    isRemotelyControlled: false,
    viewRange: null,
  },
  isFetchingDetails: false,
  resizeHeight: null,
}

export const chartReducer = createReducer<StateT>(
  {},
  initialState,
)

const getSubstate = (state: StateT, id: string) => state[id] || initialSingleState

chartReducer.on(fetchDataAction.success, (state, { id, chartData, fetchDataParams }) => ({
  ...state,
  [id]: {
    ...getSubstate(state, id),
    chartData,
    fetchDataParams,
  },
}))

chartReducer.on(fetchChartAction.request, (state, { id }) => ({
  ...state,
  [id]: {
    ...getSubstate(state, id),
    isFetchingDetails: true,
  },
}))

chartReducer.on(fetchChartAction.success, (state, { id, chartDetails }) => ({
  ...state,
  [id]: {
    ...getSubstate(state, id),
    chartDetails,
    isFetchingDetails: false,
  },
}))

// todo handle errors without creating a loop
// chartReducer.on(fetchChartAction.failure, (state, { id }) => ({
//   ...state,
//   [id]: {
//     ...getSubstate(state, id),
//     isFetchingDetails: false,
//   },
// }))

chartReducer.on(setResizeHeightAction, (state, { id, resizeHeight }) => ({
  ...state,
  [id]: {
    ...getSubstate(state, id),
    resizeHeight,
  },
}))
