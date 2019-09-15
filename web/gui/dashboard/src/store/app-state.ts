import { StateT as GlobalStateT } from "domains/global/reducer"
import { storeKey as globalKey } from "domains/global/constants"

import { StateT as ChartStateT } from "domains/chart/reducer"
import { storeKey as chartKey } from "domains/chart/constants"

export type AppStateT = {
  [globalKey]: GlobalStateT
  [chartKey]: ChartStateT
}
