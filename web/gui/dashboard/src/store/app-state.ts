import { StateT as GlobalStateT, globalKey } from "domains/global/reducer"
import { StateT as ChartStateT, chartKey } from "../domains/chart/reducer"

export type AppStateT = {
  [globalKey]: GlobalStateT
  [chartKey]: ChartStateT
}
