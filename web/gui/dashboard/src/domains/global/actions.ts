import { createAction } from "redux-act"

export const isCloudEnabled = createAction<boolean>("is cloud enabled")

interface RequestCommonColors {
  chartContext: string
  chartUuid: string
  colorsAttribute: string | undefined
  commonColorsAttribute: string | undefined
  dimensionNames: string[]
}
export const requestCommonColorsAction = createAction<RequestCommonColors>("globalRequestCommonColors")

export const setTimezoneAction = createAction<{timezone: string}>("globalSetTmezone")
