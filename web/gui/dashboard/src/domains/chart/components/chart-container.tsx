import { forEachObjIndexed, propOr } from "ramda"
import React, { useEffect, useState, useLayoutEffect } from "react"
import { useSelector, useDispatch } from "react-redux"
import { useInterval, useThrottle } from "react-use"

import { AppStateT } from "store/app-state"

import { selectGlobalPanAndZoom, selectGlobalSelection } from "domains/global/selectors"

import { chartLibrariesSettings } from "../utils/chartLibrariesSettings"
import { Attributes } from "../utils/transformDataAttributes"
import { getChartPixelsPerPoint } from "../utils/get-chart-pixels-per-point"
import { getPortalNodeStyles } from "../utils/get-portal-node-styles"

import { fetchDataAction } from "../actions"
import { selectChartData, selectChartDetails, selectChartFetchDataParams } from "../selectors"

import { Chart } from "./chart"

const getChartURLOptions = (attributes: Attributes) => {
  const {
    appendOptions,
    overrideOptions,
  } = attributes
  let ret = ""

  ret += overrideOptions
    ? overrideOptions.toString()
    : chartLibrariesSettings[attributes.chartLibrary].options(attributes)

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


  // todo local state option
  const globalPanAndZoom = useSelector(selectGlobalPanAndZoom)
  const isGlobalPanAndZoomMaster = !!globalPanAndZoom && globalPanAndZoom.masterID === chartUuid
  const shouldForceTimeRange: boolean = propOr(false, "shouldForceTimeRange", globalPanAndZoom)
  const isRemotelyControlled = !globalPanAndZoom
    || !isGlobalPanAndZoomMaster
    || shouldForceTimeRange


  const fetchDataParams = useSelector((state: AppStateT) => selectChartFetchDataParams(
    state, { id: chartUuid },
  ))

  const hoveredX = useSelector(selectGlobalSelection)
  const [shouldFetch, setShouldFetch] = useState<boolean>(true)
  useInterval(() => {
    if (!globalPanAndZoom && !hoveredX) {
      setShouldFetch(true)
    }
  }, 2000) // todo add to config

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

  const [hasPortalNodeBeenStyled, setHasPortalNodeBeenStyled] = useState<boolean>(false)
  // todo take width via hook/HOC and put this into useMemo
  const chartWidth = portalNode.getBoundingClientRect().width
    - (hasLegend(attributes) ? 140 : 0)

  /**
   * fetch data
   */
  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: chartUuid }))
  const dispatch = useDispatch()
  useEffect(() => {
    if (shouldFetch && chartDetails && hasPortalNodeBeenStyled) {
      // todo can be overriden by main.js
      const forceDataPoints = window.NETDATA.options.force_data_points

      let after
      let before
      let viewRange
      let pointsMultiplier = 1

      if (globalPanAndZoom) {
        if (isGlobalPanAndZoomMaster) {
          after = Math.round(globalPanAndZoom.after / 1000)
          before = Math.round(globalPanAndZoom.before / 1000)

          viewRange = [after, before]

          if (window.NETDATA.options.current.pan_and_zoom_data_padding) {
            const requestedPadding = Math.round((before - after) / 2)
            after -= requestedPadding
            before += requestedPadding
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

      viewRange = ((viewRange || [after, before]).map((x) => x * 1000)) as [number, number]

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
        dimensions: attributes.dimensions,

        // properties for the reducer
        fetchDataParams: {
          // we store it here so it is only available when data is fetched
          // those params should be synced with data
          isRemotelyControlled,
          viewRange,
        },
        id: chartUuid,
      }))
    }
  }, [attributes, chartDetails, chartSettings, chartUuid, chartWidth, dispatch, globalPanAndZoom,
    hasLegend, hasPortalNodeBeenStyled, initialAfter, initialBefore, isGlobalPanAndZoomMaster,
    isRemotelyControlled, portalNode, shouldFetch])

  // todo omit this for Cloud/Main Agent app
  useLayoutEffect(() => {
    const styles = getPortalNodeStyles(attributes, chartSettings)
    forEachObjIndexed((value, styleName) => {
      if (value) {
        portalNode.style.setProperty(styleName, value)
      }
    }, styles)
    // eslint-disable-next-line no-param-reassign
    portalNode.className = hasLegend ? "netdata-container-with-legend" : "netdata-container"
    setHasPortalNodeBeenStyled(true)
  }, [attributes, chartSettings, hasLegend, portalNode, setHasPortalNodeBeenStyled])


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
      isRemotelyControlled={fetchDataParams.isRemotelyControlled}
      showLatestOnBlur={!globalPanAndZoom}
    />
  )
}
