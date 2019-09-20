import { takeEvery, put, call } from "redux-saga/effects"

import { axiosInstance } from "utils/api"

import { fetchDataAction, FetchDataPayload } from "./actions"

type FetchDataSaga = { payload: FetchDataPayload }
function* fetchDataSaga({ payload }: FetchDataSaga) {
  const {
    // props for api
    chart, format, points, group, gtime, options, after, before,
    // props for the store
    fetchDataParams, id,
  } = payload
  let response
  try {
    response = yield call(axiosInstance.get, "data", {
      params: {
        chart,
        _: new Date().valueOf(),
        format,
        points,
        group,
        gtime,
        options,
        after,
        before,
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
    fetchDataParams,
    id,
  }))
}

export function* chartSagas() {
  yield takeEvery(fetchDataAction.request, fetchDataSaga)
}
