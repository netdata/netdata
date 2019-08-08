import React from "react"

import { Attributes } from "../utils/transformDataAttributes"
import { ChartData, ChartDetails } from "../chart-types"
import { ChartLibraryName } from "../utils/chartLibrariesSettings"
import { DygraphChart } from "./lib-charts/dygraph-chart"

interface Props {
  attributes: Attributes
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: ChartLibraryName
  colors: {
    [key: string]: string
  }
  chartUuid: string
  orderedColors: string[]
}

export const AbstractChart = ({
  attributes,
  chartData,
  chartDetails,
  chartLibrary,
  colors,
  chartUuid,
  orderedColors,
}: Props) => (
  <DygraphChart
    attributes={attributes}
    chartData={chartData}
    chartDetails={chartDetails}
    chartLibrary={chartLibrary}
    colors={colors}
    chartUuid={chartUuid}
    orderedColors={orderedColors}
  />
)
