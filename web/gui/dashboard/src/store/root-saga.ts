import { delay } from "redux-saga/effects"

export function* rootSaga() {
  yield delay(500)
}
