import { combineReducers } from "redux"
import { createReducer } from "redux-act"

import {
  initialState as globalInitialState,
} from "domains/global/reducer"
import { storeKey as globalStoreKey } from "domains/global/constants"

import { storeKey } from "../constants"
import { initialState, StateT } from "../reducer"

export const mockStoreWithGlobal = (state?: StateT) => (
  combineReducers({
    [storeKey]: createReducer({}, state || initialState),
    [globalStoreKey]: createReducer({}, globalInitialState),
  })
)
