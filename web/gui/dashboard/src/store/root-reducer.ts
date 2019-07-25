import { combineReducers } from "redux"
import {
  globalReducer,
  globalKey,
} from "../domains/global/reducer"

export default combineReducers({
  [globalKey]: globalReducer,
})
