import React from "react"
import classNames from "classnames"

import { Attributes } from "../utils/transformDataAttributes"
import { ChartData } from "../chart-types"

interface Props {
  chartData: ChartData
  attributes: Attributes
}

export const Chart = ({
  chartData,
  attributes: {
    legend,
    dygraphTheme,
  },
}: Props) => {
  const chartLibrary = "dygraph"
  console.log("chartData", chartData) // eslint-disable-line no-console
  const chartUuid = "chart-uuid" // todo
  const chartElemId = `${chartLibrary}-${chartUuid}-chart`

  const isSparkline = dygraphTheme === "sparkline"
  // not using __hasLegendCache__ as in old-dashboard, because performance tweaks like this
  // probably won't be needed in react app
  const hasLegend = !isSparkline && legend
  // todo make a separate getter for chart configs, where we can pass attributes, chartData
  // and in return get stuff like hasLegend
  return (
    <>
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
        <div>chart legend</div>
      )}
    </>
  )
}
