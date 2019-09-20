import React, { useCallback, useMemo } from "react"

import { setGlobalChartUnderlayAction, setGlobalPanAndZoomAction } from "domains/global/actions"
import { useDispatch } from "react-redux"
import { Attributes } from "../utils/transformDataAttributes"
import { ChartData, ChartDetails } from "../chart-types"
import { ChartLibraryName } from "../utils/chartLibrariesSettings"
import { DygraphChart } from "./lib-charts/dygraph-chart"

interface Props {
  attributes: Attributes
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: ChartLibraryName
  chartWidth: number
  colors: {
    [key: string]: string
  }
  chartUuid: string
  dimensionsVisibility: boolean[]
  isRemotelyControlled: boolean
  legendFormatValue: ((v: number) => number | string) | undefined
  orderedColors: string[]
  hoveredX: number | null
  setHoveredX: (hoveredX: number | null) => void
  setMinMax: (minMax: [number, number]) => void
  unitsCurrent: string
  viewAfter: number,
  viewBefore: number,
}

export const AbstractChart = ({
  attributes,
  chartData,
  chartDetails,
  chartLibrary,
  chartWidth,
  colors,
  chartUuid,
  dimensionsVisibility,
  isRemotelyControlled,
  legendFormatValue,
  orderedColors,
  hoveredX,
  setHoveredX,
  setMinMax,
  unitsCurrent,
  viewAfter,
  viewBefore,
}: Props) => {
  const dispatch = useDispatch()

  // old dashboard persists min duration based on first chartWidth, i assume it's a bug
  // and will update fixedMinDuration when width changes
  const fixedMinDuration = useMemo(() => (
    Math.round((chartWidth / 30) * chartDetails.update_every * 1000)
  ), [chartDetails.update_every, chartWidth])

  const updateChartPanAndZoom = useCallback(({ after, before, callback }) => {
    if (before < after) {
      return
    }
    let minDuration = fixedMinDuration

    const currentDuraton = Math.round(viewBefore - viewAfter)

    let afterForced = Math.round(after)
    let beforeForced = Math.round(before)
    const viewUpdateEvery = chartData.view_update_every * 1000

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

    // if (this.current.name === 'auto') {
    //   this.log(logme + 'caller called me with mode: ' + this.current.name);
    //   this.setMode('pan');
    // }


    // todo support force_update_at in some way
    // this.current.force_update_at = Date.now() +
    // window.NETDATA.options.current.pan_and_zoom_delay;
    // this.current.force_after_ms = after;
    // this.current.force_before_ms = before;
    dispatch(setGlobalPanAndZoomAction({
      after: afterForced,
      before: beforeForced,
      masterID: chartUuid,
    }))

    if (doCallback && typeof callback === "function") {
      callback()
    }
  }, [chartData.view_update_every, chartUuid, dispatch, fixedMinDuration, viewAfter, viewBefore])


  const setGlobalChartUnderlay = useCallback(({ after, before, masterID }) => {
    dispatch(setGlobalChartUnderlayAction({ after, before, masterID }))

    // freeze charts
    // don't send masterID, so no padding is applied
    dispatch(setGlobalPanAndZoomAction({ after: viewAfter, before: viewBefore }))
  }, [dispatch, viewAfter, viewBefore])


  return (
    <DygraphChart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartLibrary={chartLibrary}
      colors={colors}
      chartUuid={chartUuid}
      dimensionsVisibility={dimensionsVisibility}
      isRemotelyControlled={isRemotelyControlled}
      legendFormatValue={legendFormatValue}
      orderedColors={orderedColors}
      hoveredX={hoveredX}
      setGlobalChartUnderlay={setGlobalChartUnderlay}
      setHoveredX={setHoveredX}
      setMinMax={setMinMax}
      unitsCurrent={unitsCurrent}
      updateChartPanAndZoom={updateChartPanAndZoom}
    />
  )
}
