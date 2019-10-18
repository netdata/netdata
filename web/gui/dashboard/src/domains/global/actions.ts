import { createAction } from "redux-act"

import { storeKey } from "./constants"

interface RequestCommonColors {
  chartContext: string
  chartUuid: string
  colorsAttribute: string | undefined
  commonColorsAttribute: string | undefined
  dimensionNames: string[]
}
export const requestCommonColorsAction = createAction<RequestCommonColors>(
  `${storeKey}/globalRequestCommonColors`,
)

export const setTimezoneAction = createAction<{timezone: string}>(`${storeKey}/globalSetTmezone`)
window.TEMPORARY_setTimezoneAction = setTimezoneAction

interface SetGlobalSelectionAction {
  chartUuid: string | null
  hoveredX: number
}
export const setGlobalSelectionAction = createAction<SetGlobalSelectionAction>(
  `${storeKey}/setGlobalSelection`,
)

interface SetGlobalPanAndZoomAction {
  after: number
  before: number
  masterID?: string
  shouldForceTimeRange?: boolean
}
export const setGlobalPanAndZoomAction = createAction<SetGlobalPanAndZoomAction>(
  `${storeKey}/setGlobalPanAndZoom`,
)

export const resetGlobalPanAndZoomAction = createAction(`${storeKey}/resetGlobalPanAndZoomAction`)

interface SetGlobalChartUnderlayAction {
  after: number
  before: number
  masterID: string
}
export const setGlobalChartUnderlayAction = createAction<SetGlobalChartUnderlayAction>(
  `${storeKey}/setGlobalChartUnderlay`,
)

interface WindowFocusChangeAction {
  hasWindowFocus: boolean
}
export const windowFocusChangeAction = createAction<WindowFocusChangeAction>(
  `${storeKey}/windowFocusChangeAction`,
)
