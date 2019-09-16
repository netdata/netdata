import React, { useEffect, useState } from "react"
import { useSelector, useDispatch } from "react-redux"
import { useInterval, useThrottle } from "react-use"

import { AppStateT } from "store/app-state"

import { selectGlobalPanAndZoom } from "domains/global/selectors"
import { chartLibrariesSettings } from "../utils/chartLibrariesSettings"
import { Attributes } from "../utils/transformDataAttributes"
import { getChartPixelsPerPoint } from "../utils/get-chart-pixels-per-point"

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


  // todo local state option
  const globalPanAndZoom = useSelector(selectGlobalPanAndZoom)
  const isGlobalPanAndZoomMaster = !!globalPanAndZoom && globalPanAndZoom.masterID === chartUuid

  // don't send new requests too often (throttle)
  // corresponds to force_update_at in old dashboard
  // + 50 is because normal loop only happened there once per 100ms anyway..
  const globalPanAndZoomThrottled = useThrottle(globalPanAndZoom,
    window.NETDATA.options.current.pan_and_zoom_delay + 50)
  useEffect(() => {
    setShouldFetch(true)
  }, [globalPanAndZoomThrottled])

  const {
    after: initialAfter = window.NETDATA.chartDefaults.after,
    before: initialBefore = window.NETDATA.chartDefaults.before,
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
      // todo can be overriden by main.js
      const forceDataPoints = window.NETDATA.options.force_data_points

      let after
      let before
      let pointsMultiplier = 1

      let requestedPadding = 0
      if (globalPanAndZoom) {
        // if (globalPanAndZoom.before !== null && globalPanAndZoom.after !== null) {
        if (isGlobalPanAndZoomMaster) {
          before = Math.round(globalPanAndZoom.before / 1000)
          after = Math.round(globalPanAndZoom.after / 1000)

          if (window.NETDATA.options.current.pan_and_zoom_data_padding) {
            requestedPadding = Math.round((before - after) / 2)
            after -= requestedPadding
            before += requestedPadding
            requestedPadding *= 1000
            pointsMultiplier = 2
          }
        } else {
          after = Math.round(globalPanAndZoom.after / 1000)
          before = Math.round(globalPanAndZoom.before / 1000)
          pointsMultiplier = 1
        }
      } else {
        // no globalPanAndZoom
        before = initialBefore
        after = initialAfter
        pointsMultiplier = 1
      }

      const dataPoints = attributes.points
        || (Math.round(chartWidth / getChartPixelsPerPoint({ attributes, chartSettings })))
      const points = forceDataPoints || (dataPoints * pointsMultiplier)

      const group = attributes.method || window.NETDATA.chartDefaults.method

      setShouldFetch(false)
      dispatch(fetchDataAction.request({
        // properties to be passed to API
        chart: chartDetails.id,
        format: chartSettings.format,
        points,
        group,
        gtime: attributes.gtime || 0,
        options: getChartURLOptions(attributes),
        after: after || null,
        before: before || null,

        // properties for the reducer
        id: chartUuid,
      }))
    }
  }, [attributes, chartDetails, chartSettings, chartUuid, chartWidth, dispatch, globalPanAndZoom,
    hasLegend, initialAfter, initialBefore, isGlobalPanAndZoomMaster, portalNode, shouldFetch])


  if (!chartData || !chartDetails) {
    return <span>loading...</span>
  }
  return (
    <Chart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartUuid={chartUuid}
      chartWidth={chartWidth}
      portalNode={portalNode}
    />
  )
}
