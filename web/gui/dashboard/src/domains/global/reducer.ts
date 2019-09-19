import { init, last, mergeAll } from "ramda"
import { createReducer } from "redux-act"

import {
  requestCommonColorsAction,
  setGlobalChartUnderlayAction,
  setGlobalSelectionAction,
  setGlobalPanAndZoomAction,
  setTimezoneAction,
} from "./actions"

export type StateT = {
  commonColorsKeys: {
    [key: string]: { // key can be uuid, chart's context or commonColors attribute
      assigned: { // name-value of dimensions and their colors
        [dimensionName: string]: string
      }
      available: string[] // an array of colors available to be used
      custom: string[] // the array of colors defined by the user
      charts: {} // the charts linked to this todo remove
      copyTheme: boolean
    }
  }
  currentSelectionMasterId: string | null
  globalPanAndZoom: null | {
    after: number
    before: number
    masterID: string
  }
  globalChartUnderlay: null | {
    after: number
    before: number
    masterID: string
  }
  timezone: string | undefined
  hoveredX: number | null
}

const initialState = {
  commonColorsKeys: {},
  currentSelectionMasterId: null,
  globalPanAndZoom: null,
  globalChartUnderlay: null,
  timezone: window.NETDATA.options.current.timezone,
  hoveredX: null,
}

export const globalReducer = createReducer<StateT>(
  {},
  initialState,
)


export interface GetKeyArguments {
  colorsAttribute: string | undefined,
  commonColorsAttribute: string | undefined,
  chartUuid: string,
  chartContext: string,
}
export const getKeyForCommonColorsState = ({
  colorsAttribute,
  commonColorsAttribute,
  chartUuid,
  chartContext,
}: GetKeyArguments) => {
  const hasCustomColors = typeof colorsAttribute === "string" && colorsAttribute.length > 0

  // when there's commonColors attribute, share the state between all charts with that attribute
  // if not, when there are custom colors, make each chart independent
  // if not, share the same state between charts with the same context
  return commonColorsAttribute
    || (hasCustomColors ? chartUuid : chartContext)
}
const hasLastOnly = (array: string[]) => last(array) === "ONLY"
const removeLastOnly = (array: string[]) => (hasLastOnly(array) ? init(array) : array)
const createCommonColorsKeysSubstate = (
  colorsAttribute: string | undefined,
  hasCustomColors: boolean,
) => {
  const custom = hasCustomColors
    ? removeLastOnly(
      (colorsAttribute as string)
        .split(" "),
    )
    : []
  const shouldCopyTheme = hasCustomColors
    // disable copyTheme when there's "ONLY" keyword in "data-colors" attribute
    ? !hasLastOnly((colorsAttribute as string).split(" "))
    : true
  const available = [
    ...custom,
    ...(shouldCopyTheme || custom.length === 0) ? window.NETDATA.themes.current.colors : [],
  ]
  return {
    assigned: {},
    available,
    custom,
  }
}

globalReducer.on(requestCommonColorsAction, (state, {
  chartContext,
  chartUuid,
  colorsAttribute,
  commonColorsAttribute,
  dimensionNames,
}) => {
  const keyName = getKeyForCommonColorsState({
    colorsAttribute, commonColorsAttribute, chartUuid, chartContext,
  })

  const hasCustomColors = typeof colorsAttribute === "string" && colorsAttribute.length > 0
  const subState = state.commonColorsKeys[keyName]
    || createCommonColorsKeysSubstate(colorsAttribute, hasCustomColors)

  const currentlyAssignedNr = Object.keys(subState.assigned).length
  const requestedDimensionsAssigned = mergeAll(
    dimensionNames
      // dont assign already assigned dimensions
      .filter((dimensionName) => !subState.assigned[dimensionName])
      .map((dimensionName, i) => ({
        [dimensionName]: subState.available[(i + currentlyAssignedNr) % subState.available.length],
      })),
  )
  const assigned = {
    ...subState.assigned,
    ...requestedDimensionsAssigned,
  }

  return {
    ...state,
    commonColorsKeys: {
      ...state.commonColorsKeys,
      [keyName]: {
        ...subState,
        assigned,
      },
    },
  }
})

globalReducer.on(setTimezoneAction, (state, { timezone = "default" }) => ({
  ...state,
  timezone,
}))

globalReducer.on(setGlobalSelectionAction, (state, { chartUuid, hoveredX }) => ({
  ...state,
  hoveredX,
  currentSelectionMasterId: chartUuid,
}))

globalReducer.on(setGlobalPanAndZoomAction, (state, { after, before, masterID }) => ({
  ...state,
  globalPanAndZoom: {
    after,
    before,
    masterID,
  },
}))

globalReducer.on(setGlobalChartUnderlayAction, (state, { after, before, masterID }) => ({
  ...state,
  globalChartUnderlay: {
    after,
    before,
    masterID,
  },
}))
