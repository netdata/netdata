import { __, prop } from "ramda"
import React, {
  useEffect, useState, useCallback, useMemo,
} from "react"
import { useDispatch, useSelector } from "react-redux"

import { requestCommonColorsAction, setGlobalSelectionAction } from "domains/global/actions"
import { createSelectAssignedColors, selectGlobalSelection } from "domains/global/selectors"
import { AppStateT } from "store/app-state"

import { Attributes } from "../utils/transformDataAttributes"
import { chartLibrariesSettings } from "../utils/chartLibrariesSettings"
import { useFormatters } from "../utils/formatters"
import { ChartData, ChartDetails } from "../chart-types"
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
  const handleSetHoveredX = useCallback((newHoveredX) => {
    if (isGlobalSelectionSyncFlagTrue) {
      dispatch(setGlobalSelectionAction({ chartUuid, hoveredX: newHoveredX }))
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
        chartWidth={chartWidth}
        dimensionsVisibility={dimensionsVisibility}
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
          chartData={chartData}
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
        <LegendToolbox />
      )}
      {window.NETDATA.options.current.resize_charts && (
        <ResizeHandler />
      )}
    </>
  )
}
