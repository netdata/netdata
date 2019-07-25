import { StateT as GlobalStateT, globalKey } from "domains/global/reducer"

export type AppStateT = {
  [globalKey]: GlobalStateT
}
