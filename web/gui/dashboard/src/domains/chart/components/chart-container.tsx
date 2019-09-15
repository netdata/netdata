import React, { useEffect, useState } from "react"
import { useSelector, useDispatch } from "react-redux"
import { useInterval } from "react-use"

import { AppStateT } from "store/app-state"

import { chartLibrariesSettings } from "../utils/chartLibrariesSettings"
import { Attributes } from "../utils/transformDataAttributes"

import { fetchDataAction } from "../actions"
import { selectChartData, selectChartDetails } from "../selectors"

import { Chart } from "./chart"

const getChartURLOptions = (attributes: Attributes) => {
  const {
    appendOptions,
    overrideOptions,
  } = attributes
  let ret = ""

  ret += overrideOptions
    ? overrideOptions.toString()
    : chartLibrariesSettings.dygraph.options(attributes)

  if (typeof appendOptions === "string") {
    ret += `|${encodeURIComponent(appendOptions)}`
  }

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


export type Props = {
  attributes: Attributes
  chartUuid: string
  portalNode: HTMLElement
}

export const ChartContainer = ({
  attributes,
  chartUuid,
  portalNode,
}: Props) => {
  const chartDetails = useSelector((state: AppStateT) => selectChartDetails(
    state, { id: chartUuid },
  ))

  const [shouldFetch, setShouldFetch] = useState<boolean>(true)
  useInterval(() => {
    setShouldFetch(true)
  }, 2000)


  const {
    after = window.NETDATA.chartDefaults.after,
    before = window.NETDATA.chartDefaults.before,
  } = attributes

  const chartSettings = chartLibrariesSettings[attributes.chartLibrary]
  const { hasLegend } = chartSettings
  // todo take width via hook/HOC and put this into useMemo
  const chartWidth = portalNode.getBoundingClientRect().width
    - (hasLegend ? 140 : 0)

  /**
   * fetch data
   */
  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: chartUuid }))
  const dispatch = useDispatch()
  useEffect(() => {
    if (shouldFetch && chartDetails) {
      const group = attributes.method || window.NETDATA.chartDefaults.method

      setShouldFetch(false)
      dispatch(fetchDataAction.request({
        // properties to be passed to API
        chart: chartDetails.id,
        format: chartSettings.format,
        points: 63,
        group,
        gtime: attributes.gtime || 0,
        options: getChartURLOptions(attributes),
        after: after || null,
        before: before || null,

        // properties for the reducer
        id: chartUuid,
      }))
    }
  }, [after, attributes, before, chartDetails, chartSettings, chartUuid, chartWidth, dispatch,
    hasLegend, portalNode, shouldFetch])


  if (!chartData || !chartDetails) {
    return <span>loading...</span>
  }
  return (
    <Chart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartUuid={chartUuid}
      portalNode={portalNode}
    />
  )
}
