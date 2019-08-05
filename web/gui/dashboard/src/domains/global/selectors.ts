import { AppStateT } from "store/app-state"
import { GetKeyArguments, getKeyForCommonColorsState, globalKey } from "./reducer"

export const createSelectAssignedColors = (args: GetKeyArguments) => (state: AppStateT) => {
  const keyName = getKeyForCommonColorsState(args)
  const substate = state[globalKey].commonColorsKeys[keyName]
  return substate.assigned
}
