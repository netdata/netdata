import { createSelector } from "reselect"

import { AppStateT } from "store/app-state"

import { GetKeyArguments, getKeyForCommonColorsState, globalKey } from "./reducer"

export const createSelectAssignedColors = (args: GetKeyArguments) => (state: AppStateT) => {
  const keyName = getKeyForCommonColorsState(args)
  const substate = state[globalKey].commonColorsKeys[keyName]
  return substate && substate.assigned
}

export const selectGlobal = (state: AppStateT) => state.global

export const selectTimezone = createSelector(
  selectGlobal,
  subState => subState.timezone,
)
