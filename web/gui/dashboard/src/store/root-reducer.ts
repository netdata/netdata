import { combineReducers } from "redux"
import { globalReducer } from "domains/global/reducer"
import { storeKey as globalKey } from "domains/global/constants"

import { chartReducer } from "domains/chart/reducer"
import { storeKey as chartKey } from "domains/chart/constants"

export default combineReducers({
  [globalKey]: globalReducer,
  [chartKey]: chartReducer,
})
