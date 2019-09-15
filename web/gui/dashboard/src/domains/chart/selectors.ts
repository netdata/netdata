import { createSelector } from "reselect"

import { AppStateT } from "store/app-state"
import { initialSingleState } from "./reducer"
import { storeKey } from "./constants"

export const selectChartsState = (state: AppStateT) => state[storeKey]
export const selectSingleChartState = createSelector(
  selectChartsState,
  (_: any, { id }: { id: string }) => id,
  (chartsState, id) => chartsState[id] || initialSingleState,
)

export const selectChartData = createSelector(
  selectSingleChartState,
  (chartState) => chartState.chartData,
)

export const selectChartDetails = createSelector(
  selectSingleChartState,
  (chartState) => chartState.chartDetails,
)
