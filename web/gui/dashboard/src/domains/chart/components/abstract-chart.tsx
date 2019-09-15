import React, { useCallback, useMemo } from "react"

import { setGlobalPanAndZoomAction } from "domains/global/actions"
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
  legendFormatValue: ((v: number) => number | string) | undefined
  orderedColors: string[]
  hoveredX: number | null
  setHoveredX: (hoveredX: number | null) => void
  setMinMax: (minMax: [number, number]) => void
  unitsCurrent: string
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
  legendFormatValue,
  orderedColors,
  hoveredX,
  setHoveredX,
  setMinMax,
  unitsCurrent,
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
    // similar naming to old dashboard, todo rethink

    const viewAfter = chartData.after * 1000
    const viewBefore = chartData.before * 1000
    const currentDuraton = Math.round(viewBefore - viewAfter)

    let afterForced = Math.round(after)
    let beforeForced = Math.round(before)

    // align them to update_every
    // stretching them further away
    afterForced -= afterForced % (chartData.view_update_every * 1000)
    beforeForced += chartData.view_update_every - (beforeForced % chartData.view_update_every)

    // the final wanted duration
    let wantedDuration = beforeForced - afterForced;

    // to allow panning, accept just a point below our minimum
    if ((currentDuraton - chartData.view_update_every) < minDuration) {
      minDuration = currentDuraton - chartData.view_update_every;
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

    const tolerance = chartData.view_update_every * 2
    const movement = Math.abs(beforeForced - chartData.view_update_every)

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
  }, [chartData, chartUuid, dispatch, fixedMinDuration])

  return (
    <DygraphChart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartLibrary={chartLibrary}
      colors={colors}
      chartUuid={chartUuid}
      dimensionsVisibility={dimensionsVisibility}
      legendFormatValue={legendFormatValue}
      orderedColors={orderedColors}
      hoveredX={hoveredX}
      setHoveredX={setHoveredX}
      setMinMax={setMinMax}
      unitsCurrent={unitsCurrent}
      updateChartPanAndZoom={updateChartPanAndZoom}
    />
  )
}
