import { takeEvery, put, call } from "redux-saga/effects"

import { axiosInstance } from "utils/api"

import {
  fetchDataAction, FetchDataPayload,
  fetchChartAction, FetchChartPayload,
} from "./actions"

type FetchDataSaga = { payload: FetchDataPayload }
function* fetchDataSaga({ payload }: FetchDataSaga) {
  const {
    // props for api
    chart, format, points, group, gtime, options, after, before, dimensions,
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
        dimensions,
      },
    })
  } catch (e) {
    console.warn("fetch chart data failure") // eslint-disable-line no-console
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

type FetchChartSaga = { payload: FetchChartPayload }
function* fetchChartSaga({ payload }: FetchChartSaga) {
  const { chart, id } = payload
  let response
  try {
    response = yield call(axiosInstance.get, "chart", {
      params: {
        chart,
      },
    })
  } catch (e) {
    console.warn("fetch chart details failure") // eslint-disable-line no-console
    yield put(fetchChartAction.failure())
    return
  }
  yield put(fetchChartAction.success({
    chartDetails: response.data,
    id,
  }))
}

export function* chartSagas() {
  yield takeEvery(fetchDataAction.request, fetchDataSaga)
  yield takeEvery(fetchChartAction.request, fetchChartSaga)
}
