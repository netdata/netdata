import { combineReducers } from "redux"
import {
  globalReducer,
  globalKey,
} from "../domains/global/reducer"

import {
  chartReducer,
  chartKey,
} from "../domains/chart/reducer"

export default combineReducers({
  [globalKey]: globalReducer,
  [chartKey]: chartReducer,
})
