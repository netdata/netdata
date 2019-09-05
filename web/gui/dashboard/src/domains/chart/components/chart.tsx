import { __, forEachObjIndexed, prop } from "ramda"
import React, {
  useEffect, useLayoutEffect, useState, useCallback, useMemo,
} from "react"
import { useDispatch, useSelector } from "react-redux"

import { requestCommonColorsAction, setGlobalSelectionAction } from "domains/global/actions"
import { createSelectAssignedColors, selectGlobalSelection } from "domains/global/selectors"

import { ChartLegend } from "./chart-legend"
import { Attributes } from "../utils/transformDataAttributes"
import { chartLibrariesSettings, ChartLibraryConfig } from "../utils/chartLibrariesSettings"
import { ChartData, ChartDetails } from "../chart-types"
import { LegendToolbox } from "./legend-toolbox"
import { ResizeHandler } from "./resize-handler"
import { AbstractChart } from "./abstract-chart"
import { useFormatters } from "../utils/formatters"

interface Props {
  chartData: ChartData
  chartDetails: ChartDetails
  chartUuid: string
  attributes: Attributes
  portalNode: HTMLElement
}

const getStyles = (attributes: Attributes, chartSettings: ChartLibraryConfig) => {
  let width
  if (typeof attributes.width === "string") {
    // eslint-disable-next-line prefer-destructuring
    width = attributes.width
  } else if (typeof attributes.width === "number") {
    width = `${attributes.width.toString()}px`
  }
  let height
  if (chartSettings.aspectRatio === undefined) {
    if (typeof attributes.height === "string") {
      // eslint-disable-next-line prefer-destructuring
      height = attributes.height
    } else if (typeof attributes.height === "number") {
      height = `${attributes.height.toString()}px`
    }
  }
  const minWidth = window.NETDATA.chartDefaults.min_width !== null
    ? window.NETDATA.chartDefaults.min_width
    : undefined
  return {
    height,
    width,
    minWidth,
  }
}

export const Chart = ({
  chartData,
  chartDetails,
  chartUuid,
  attributes: {
    chartLibrary,
  },
  attributes,
  portalNode,
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

  // todo omit this for Cloud/Main Agent app
  useLayoutEffect(() => {
    const styles = getStyles(attributes, chartSettings)
    forEachObjIndexed((value, styleName) => {
      if (value) {
        portalNode.style.setProperty(styleName, value)
      }
    }, styles)
    // eslint-disable-next-line no-param-reassign
    portalNode.className = hasLegend ? "netdata-container-with-legend" : "netdata-container"
  }, [attributes, chartSettings, hasLegend, portalNode])

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

  const selectAssignedColors = createSelectAssignedColors({
    chartContext: chartDetails.context,
    chartUuid,
    colorsAttribute: attributes.colors,
    commonColorsAttribute: attributes.commonColors,
  })
  const colors = useSelector(selectAssignedColors)
  if (!colors) {
    return null
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
        legendFormatValue={legendFormatValue}
        orderedColors={orderedColors}
        hoveredX={hoveredX}
        setHoveredX={handleSetHoveredX}
        setMinMax={([min, max]) => { legendFormatValueDecimalsFromMinMax(min, max) }}
        unitsCurrent={unitsCurrent}
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
