import React, { useEffect } from "react"
import classNames from "classnames"
import { useDispatch, useSelector } from "react-redux"

import { requestCommonColorsAction } from "domains/global/actions"
import { createSelectAssignedColors } from "domains/global/selectors"

import { ChartLegend } from "./chart-legend"
import { Attributes } from "../utils/transformDataAttributes"
import { chartLibrariesSettings, ChartLibraryConfig } from "../utils/chartLibrariesSettings"
import { ChartData, ChartDetails } from "../chart-types"
import { LegendToolbox } from "./legend-toolbox"
import { ResizeHandler } from "./resize-handler"

interface Props {
  chartData: ChartData
  chartDetails: ChartDetails
  chartUuid: string
  attributes: Attributes
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
  if (chartSettings === undefined) {
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
}: Props) => {
  const chartElemId = `${chartLibrary}-${chartUuid}-chart`

  const chartSettings = chartLibrariesSettings[chartLibrary]
  const { hasLegend } = chartSettings

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
    chartData.dimension_names.join(("")),
  ])

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

  return (
    <div
      style={getStyles(attributes, chartSettings)}
      className={hasLegend ? "netdata-container-with-legend" : "netdata-container"}
    >
      <div
        id={chartElemId}
        className={hasLegend
          ? classNames(
            "netdata-chart-with-legend-right",
            `netdata-${chartLibrary}-chart-with-legend-right`,
          )
          : classNames(
            "netdata-chart",
            `netdata-${chartLibrary}-chart`,
          )
        }
      />
      {hasLegend && (
        <ChartLegend
          attributes={attributes}
          chartData={chartData}
          chartDetails={chartDetails}
          chartLibrary={chartLibrary}
          colors={colors}
        />
      )}
      {shouldDisplayToolbox && (
        <LegendToolbox />
      )}
      {window.NETDATA.options.current.resize_charts && (
        <ResizeHandler />
      )}
    </div>
  )
}
