import { createReducer } from "redux-act"
import { isCloudEnabled } from "./actions"

export type StateT = {
  isCloudEnabled: boolean,
}

const initialState = {
  isCloudEnabled: false,
}

export const globalReducer = createReducer<StateT>(
  {},
  initialState,
)

globalReducer.on(isCloudEnabled, (state, payload) => ({
  ...state,
  isCloudEnabled: payload,
}))

export const globalKey = "global"
