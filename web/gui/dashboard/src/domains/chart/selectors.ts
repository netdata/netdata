import { prop } from "ramda"
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

const selectChartDetails = createSelector(selectSingleChartState, prop("chartDetails"))
const selectIsFetchingDetails = createSelector(selectSingleChartState, prop("isFetchingDetails"))

export const selectChartDetailsRequest = createSelector(
  selectChartDetails,
  selectIsFetchingDetails,
  (chartDetails, isFetchingDetails) => ({ chartDetails, isFetchingDetails }),
)

export const selectChartViewRange = createSelector(
  selectSingleChartState,
  (chartState) => chartState.fetchDataParams.viewRange,
)

export const selectChartFetchDataParams = createSelector(
  selectSingleChartState,
  (chartState) => chartState.fetchDataParams,
)
