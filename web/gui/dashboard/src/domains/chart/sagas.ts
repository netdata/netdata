import { takeEvery, put, call } from "redux-saga/effects"

import { axiosInstance } from "utils/api"

import { chartLibrariesSettings } from "./utils/chartLibrariesSettings"
import { Attributes } from "./utils/transformDataAttributes"

import { fetchDataAction, FetchDataPayload } from "./actions"

const getChartURLOptions = (attributes: Attributes) => {
  let ret = ""

  // todo (support attribute)
  // if (this.override_options !== null) {
  //   ret = this.override_options.toString()
  // } else {
  //   ret = this.library.options(this)
  // }
  ret += chartLibrariesSettings.dygraph.options(attributes)

  // todo (support attribute)
  // if (this.append_options !== null) {
  //   ret += `%7C${this.append_options.toString()}`
  // }

  ret += "|jsonwrap"

  // todo
  const isForUniqueId = false
  // always add `nonzero` when it's used to create a chartDataUniqueID
  // we cannot just remove `nonzero` because of backwards compatibility with old snapshots
  if (isForUniqueId || window.NETDATA.options.current.eliminate_zero_dimensions) {
    ret += "|nonzero"
  }

  return ret
}

type FetchDataSaga = { payload: FetchDataPayload }
function* fetchDataSaga({ payload: { attributes, id, uuid } }: FetchDataSaga) {
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
        options: getChartURLOptions(attributes),
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
