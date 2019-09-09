import { spawn } from "redux-saga/effects"

import { chartSagas } from "domains/chart/sagas"

export function* rootSaga() {
  yield spawn(chartSagas)
}
