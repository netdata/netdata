import { prop } from "ramda"
import { createSelector } from "reselect"

import { AppStateT } from "store/app-state"

import { GetKeyArguments, getKeyForCommonColorsState } from "./reducer"
import { storeKey } from "./constants"

export const createSelectAssignedColors = (args: GetKeyArguments) => (state: AppStateT) => {
  const keyName = getKeyForCommonColorsState(args)
  const substate = state[storeKey].commonColorsKeys[keyName]
  return substate && substate.assigned
}

export const selectGlobal = (state: AppStateT) => state.global

export const selectTimezone = createSelector(
  selectGlobal,
  (subState) => subState.timezone,
)

export const selectGlobalSelection = createSelector(
  selectGlobal,
  prop("hoveredX"),
)

export const selectGlobalSelectionMaster = createSelector(
  selectGlobal,
  prop("currentSelectionMasterId"),
)

export const selectGlobalPanAndZoom = createSelector(
  selectGlobal,
  prop("globalPanAndZoom"),
)
