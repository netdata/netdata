import React from "react"
import classNames from "classnames"

import { ChartLegend } from "./chart-legend"
import { Attributes } from "../utils/transformDataAttributes"
import { chartLibrariesSettings, ChartLibraryConfig } from "../utils/chartLibrariesSettings"
import { ChartData, ChartDetails } from "../chart-types"
import { LegendToolbox } from "./legend-toolbox"
import { ResizeHandler } from "./resize-handler"

interface Props {
  chartData: ChartData
  chartDetails: ChartDetails
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
  attributes: {
    chartLibrary,
  },
  attributes,
}: Props) => {
  console.log("chartData", chartData) // eslint-disable-line no-console
  const chartUuid = "chart-uuid" // todo
  const chartElemId = `${chartLibrary}-${chartUuid}-chart`

  const chartSettings = chartLibrariesSettings[chartLibrary]
  const { hasLegend } = chartSettings

  const shouldDisplayToolbox = hasLegend(attributes)
    && window.NETDATA.options.current.legend_toolbox
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
          chartDetails={chartDetails}
          chartLibrary={chartLibrary}
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
