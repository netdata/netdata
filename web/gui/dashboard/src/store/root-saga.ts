import { spawn } from "redux-saga/effects"

import { chartSagas } from "domains/chart/sagas"
import { globalSagas } from "domains/global/sagas"

export function* rootSaga() {
  yield spawn(globalSagas)
  yield spawn(chartSagas)
}
