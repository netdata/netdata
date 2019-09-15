import { combineReducers } from "redux"
import { globalReducer } from "domains/global/reducer"
import { storeKey as globalKey } from "domains/global/constants"

import {
  chartReducer,
  chartKey,
} from "../domains/chart/reducer"

export default combineReducers({
  [globalKey]: globalReducer,
  [chartKey]: chartReducer,
})
