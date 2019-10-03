import { __, prop } from "ramda"
import React, {
  useEffect, useState, useCallback, useMemo,
} from "react"
import { useDispatch, useSelector } from "react-redux"

import {
  requestCommonColorsAction,
  resetGlobalPanAndZoomAction,
  setGlobalPanAndZoomAction,
  setGlobalSelectionAction,
} from "domains/global/actions"
import { createSelectAssignedColors, selectGlobalSelection } from "domains/global/selectors"
import { AppStateT } from "store/app-state"

import { getPanAndZoomStep } from "../utils/get-pan-and-zoom-step"
import { Attributes } from "../utils/transformDataAttributes"
import { chartLibrariesSettings } from "../utils/chartLibrariesSettings"
import { useFormatters } from "../utils/formatters"
import { ChartData, ChartDetails, DygraphData } from "../chart-types"
import { selectChartViewRange } from "../selectors"

import { ChartLegend } from "./chart-legend"
import { LegendToolbox } from "./legend-toolbox"
import { ResizeHandler } from "./resize-handler"
import { AbstractChart } from "./abstract-chart"

interface Props {
  chartData: ChartData
  chartDetails: ChartDetails
  chartUuid: string
  chartWidth: number
  attributes: Attributes
  isRemotelyControlled: boolean
}

export const Chart = ({
  attributes,
  attributes: {
    chartLibrary,
  },
  chartData,
  chartDetails,
  chartUuid,
  chartWidth,
  isRemotelyControlled,
}: Props) => {
  const chartSettings = chartLibrariesSettings[chartLibrary]
  const { hasLegend } = chartSettings
  const {
    units = chartDetails.units,
    unitsCommon,
    unitsDesired = window.NETDATA.options.current.units,
  } = attributes

  // selecting dimensions
  const dimensionNamesFlatString = chartData.dimension_names.join("")
  // we need to have empty selectedDimensions work as {all enabled}, in case
  // new dimensions show up (when all are enabled, the new dimensions should also auto-enable)
  const [selectedDimensions, setSelectedDimensions] = useState<string[]>([])
  const dimensionsVisibility = useMemo(() => chartData.dimension_names.map(
    (dimensionName) => (selectedDimensions.length === 0
      ? true
      : selectedDimensions.includes(dimensionName)),
  ),
  [chartData.dimension_names, selectedDimensions])


  const shouldDisplayToolbox = hasLegend(attributes)
    && window.NETDATA.options.current.legend_toolbox

  const dispatch = useDispatch()
  useEffect(() => {
    dispatch(requestCommonColorsAction({
      chartContext: chartDetails.context,
      chartUuid,
      colorsAttribute: attributes.colors,
      commonColorsAttribute: attributes.commonColors,
      dimensionNames: chartData.dimension_names,
    }))
  }, [ // eslint-disable-line react-hooks/exhaustive-deps
    dispatch, attributes.commonColors, chartDetails.context, chartUuid, attributes.colors,
    dimensionNamesFlatString,
  ])

  const {
    legendFormatValue,
    legendFormatValueDecimalsFromMinMax,
    unitsCurrent,
  } = useFormatters({
    attributes,
    data: chartData,
    units,
    unitsCommon,
    unitsDesired,
    uuid: chartUuid,
  })

  const [localHoveredX, setLocalHoveredX] = useState<number | null>(null)

  const isGlobalSelectionSyncFlagTrue = true // todo
  const handleSetHoveredX = useCallback((newHoveredX, noMaster) => {
    if (isGlobalSelectionSyncFlagTrue) {
      const action = noMaster
        ? { chartUuid: null, hoveredX: newHoveredX }
        : { chartUuid, hoveredX: newHoveredX }
      dispatch(setGlobalSelectionAction(action))
    } else {
      setLocalHoveredX(newHoveredX)
    }
  }, [chartUuid, dispatch, isGlobalSelectionSyncFlagTrue])
  const globalHoveredX = useSelector(selectGlobalSelection)
  const hoveredX = isGlobalSelectionSyncFlagTrue
    ? globalHoveredX
    : localHoveredX

  const viewRange = useSelector((state: AppStateT) => selectChartViewRange(
    state, { id: chartUuid },
  ))
  const viewAfter = Math.max(chartData.after * 1000, viewRange[0])
  const viewBefore = viewRange[1] > 0
    ? Math.min(chartData.before * 1000, viewRange[1])
    : chartData.before * 1000 // when 'before' is 0 or negative

  const netdataFirst = chartData.first_entry * 1000
  const netdataLast = chartData.last_entry * 1000

  // old dashboard persists min duration based on first chartWidth, i assume it's a bug
  // and will update fixedMinDuration when width changes
  const fixedMinDuration = useMemo(() => (
    Math.round((chartWidth / 30) * chartDetails.update_every * 1000)
  ), [chartDetails.update_every, chartWidth])


  /**
   * pan-and-zoom handler (both for toolbox and mouse events)
   */
  const handleUpdateChartPanAndZoom = useCallback(({
    after, before, callback, shouldForceTimeRange, shouldNotExceedAvailableRange,
  }) => {
    if (before < after) {
      return
    }
    let minDuration = fixedMinDuration

    const currentDuraton = Math.round(viewBefore - viewAfter)

    let afterForced = Math.round(after)
    let beforeForced = Math.round(before)
    const viewUpdateEvery = chartData.view_update_every * 1000

    if (shouldNotExceedAvailableRange) {
      const first = netdataFirst + viewUpdateEvery
      const last = netdataLast + viewUpdateEvery
      // first check "before"
      if (beforeForced > last) {
        afterForced -= (before - last)
        beforeForced = last
      }

      if (afterForced < first) {
        afterForced = first
      }
    }


    // align them to update_every
    // stretching them further away
    afterForced -= afterForced % (viewUpdateEvery)
    beforeForced += viewUpdateEvery - (beforeForced % viewUpdateEvery)

    // the final wanted duration
    let wantedDuration = beforeForced - afterForced

    // to allow panning, accept just a point below our minimum
    if ((currentDuraton - viewUpdateEvery) < minDuration) {
      minDuration = currentDuraton - viewUpdateEvery
    }

    // we do it, but we adjust to minimum size and return false
    // when the wanted size is below the current and the minimum
    // and we zoom
    let doCallback = true
    if (wantedDuration < currentDuraton && wantedDuration < minDuration) {
      minDuration = fixedMinDuration

      const dt = (minDuration - wantedDuration) / 2
      beforeForced += dt
      afterForced -= dt
      wantedDuration = beforeForced - afterForced
      doCallback = false
    }

    const tolerance = viewUpdateEvery * 2
    const movement = Math.abs(beforeForced - viewBefore)

    if (
      Math.abs(currentDuraton - wantedDuration) <= tolerance && movement <= tolerance && doCallback
    ) {
      return
    }

    dispatch(setGlobalPanAndZoomAction({
      after: afterForced,
      before: beforeForced,
      masterID: chartUuid,
      shouldForceTimeRange,
    }))

    if (doCallback && typeof callback === "function") {
      callback(afterForced, beforeForced)
    }
  }, [chartData.view_update_every, chartUuid, dispatch, fixedMinDuration, netdataFirst,
    netdataLast, viewAfter, viewBefore])


  /**
   * toolbox handlers
   */
  const handleToolBoxPanAndZoom = useCallback((after: number, before: number) => {
    const newAfter = Math.max(after, netdataFirst)
    const newBefore = Math.min(before, netdataLast)
    handleUpdateChartPanAndZoom({
      after: newAfter,
      before: newBefore,
      shouldForceTimeRange: true,
    })
  }, [handleUpdateChartPanAndZoom, netdataFirst, netdataLast])

  const handleToolboxLeftClick = useCallback((event: React.MouseEvent) => {
    const step = (viewBefore - viewAfter) * getPanAndZoomStep(event)
    const newBefore = viewBefore - step
    const newAfter = viewAfter - step
    if (newAfter >= netdataFirst) {
      handleToolBoxPanAndZoom(newAfter, newBefore)
    }
  }, [handleToolBoxPanAndZoom, netdataFirst, viewAfter, viewBefore])

  const handleToolboxRightClick = useCallback((event: React.MouseEvent) => {
    const timeWindow = viewBefore - viewAfter
    const step = timeWindow * getPanAndZoomStep(event)
    const newBefore = Math.min(viewBefore + step, netdataLast)
    const newAfter = newBefore - timeWindow
    handleToolBoxPanAndZoom(newAfter, newBefore)
  }, [handleToolBoxPanAndZoom, netdataLast, viewAfter, viewBefore])

  const handleToolboxZoomInClick = useCallback((event: React.MouseEvent) => {
    const dt = ((viewBefore - viewAfter) * getPanAndZoomStep(event) * 0.8) / 2
    const newAfter = viewAfter + dt
    const newBefore = viewBefore - dt
    handleToolBoxPanAndZoom(newAfter, newBefore)
  }, [handleToolBoxPanAndZoom, viewAfter, viewBefore])

  const handleToolboxZoomOutClick = useCallback((event: React.MouseEvent) => {
    const dt = ((viewBefore - viewAfter) / (1.0 - (getPanAndZoomStep(event) * 0.8))
      - (viewBefore - viewAfter)) / 2
    const newAfter = viewAfter - dt
    const newBefore = viewBefore + dt
    handleToolBoxPanAndZoom(newAfter, newBefore)
  }, [handleToolBoxPanAndZoom, viewAfter, viewBefore])

  const handleToolboxResetClick = useCallback(() => {
    dispatch(resetGlobalPanAndZoomAction())
  }, [dispatch])


  /**
   * assign colors
   */
  const selectAssignedColors = createSelectAssignedColors({
    chartContext: chartDetails.context,
    chartUuid,
    colorsAttribute: attributes.colors,
    commonColorsAttribute: attributes.commonColors,
  })
  const colors = useSelector(selectAssignedColors)
  if (!colors) {
    return null // wait for createSelectAssignedColors reducer result to come back
  }
  const orderedColors = chartData.dimension_names.map(prop(__, colors))

  return (
    <>
      <AbstractChart
        attributes={attributes}
        chartData={chartData}
        chartDetails={chartDetails}
        chartLibrary={chartLibrary}
        colors={colors}
        chartUuid={chartUuid}
        dimensionsVisibility={dimensionsVisibility}
        onUpdateChartPanAndZoom={handleUpdateChartPanAndZoom}
        isRemotelyControlled={isRemotelyControlled}
        legendFormatValue={legendFormatValue}
        orderedColors={orderedColors}
        hoveredX={hoveredX}
        setHoveredX={handleSetHoveredX}
        setMinMax={([min, max]) => { legendFormatValueDecimalsFromMinMax(min, max) }}
        unitsCurrent={unitsCurrent}
        viewAfter={viewAfter}
        viewBefore={viewBefore}
      />
      {hasLegend && (
        <ChartLegend
          attributes={attributes}
          chartData={chartData as DygraphData}
          chartDetails={chartDetails}
          chartLibrary={chartLibrary}
          colors={colors}
          hoveredX={hoveredX}
          legendFormatValue={legendFormatValue}
          selectedDimensions={selectedDimensions}
          setSelectedDimensions={setSelectedDimensions}
          unitsCurrent={unitsCurrent}
          viewBefore={viewBefore}
        />
      )}
      {shouldDisplayToolbox && (
        <LegendToolbox
          onToolboxLeftClick={handleToolboxLeftClick}
          onToolboxResetClick={handleToolboxResetClick}
          onToolboxRightClick={handleToolboxRightClick}
          onToolboxZoomInClick={handleToolboxZoomInClick}
          onToolboxZoomOutClick={handleToolboxZoomOutClick}
        />
      )}
      {window.NETDATA.options.current.resize_charts && (
        <ResizeHandler />
      )}
    </>
  )
}
