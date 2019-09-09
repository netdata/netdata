import { takeEvery, put, call } from "redux-saga/effects"

import { axiosInstance } from "utils/api"

import { fetchDataAction, FetchDataPayload } from "./actions"

type FetchDataSaga = { payload: FetchDataPayload }
function* fetchDataSaga({ payload: { id, uuid } }: FetchDataSaga) {
  let response
  try {
    response = yield call(axiosInstance.get, "data", {
      params: {
        chart: id,
        _: new Date().valueOf(),
        // todo support overriding with attributes
        format: "json", // todo get from chart settings
        points: 63, // todo
        group: window.NETDATA.chartDefaults.method,
        gtime: 0,
        after: -300,
      },
    })
  } catch (e) {
    yield put(fetchDataAction.failure())
    // todo implement error handling to support NETDATA.options.current.retries_on_data_failures
    return
  }
  // todo do xss check of data
  yield put(fetchDataAction.success({
    chartData: response.data,
    id: uuid,
  }))
}

export function* chartSagas() {
  yield takeEvery(fetchDataAction.request, fetchDataSaga)
}
