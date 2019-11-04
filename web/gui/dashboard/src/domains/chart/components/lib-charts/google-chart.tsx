import React, {
  useRef, useState, useLayoutEffect,
} from "react"

import { Attributes } from "domains/chart/utils/transformDataAttributes"
import { ChartDetails, EasyPieChartData } from "domains/chart/chart-types"
import { loadGoogleVisualizationApi } from "domains/chart/utils/google-visualization-loader"

interface Props {
  attributes: Attributes
  chartData: EasyPieChartData
  chartDetails: ChartDetails
  chartElementClassName: string
  chartElementId: string
  orderedColors: string[]
  unitsCurrent: string
}
export const GoogleChart = ({
  attributes,
  chartData,
  chartDetails,
  chartElementClassName,
  chartElementId,
  orderedColors,
  unitsCurrent,
}: Props) => {
  const chartElement = useRef<HTMLDivElement>(null)
  const googleChartInstance = useRef<
    google.visualization.AreaChart |
    google.visualization.LineChart>()

  const [hasApiBeenLoaded, setHasApiBeenLoaded] = useState(false)
  loadGoogleVisualizationApi()
    .then(() => {
      setHasApiBeenLoaded(true)
    })

  const googleOptions = useRef<{[key: string]: unknown}>()

  // update chart
  useLayoutEffect(() => {
    if (googleChartInstance.current && googleOptions.current) {
      const dataTable = new window.google.visualization.DataTable(chartData.result)
      googleChartInstance.current.draw(dataTable, googleOptions.current)
    }
  }, [chartData])

  // create chart
  useLayoutEffect(() => {
    if (chartElement.current && !googleOptions.current && hasApiBeenLoaded) {
      const dataTable = new window.google.visualization.DataTable(chartData.result)

      const {
        title = chartDetails.title,
      } = attributes
      const chartType = chartDetails.chart_type
      const areaOpacity = new Map([
        ["area", window.NETDATA.options.current.color_fill_opacity_area],
        ["stacked", window.NETDATA.options.current.color_fill_opacity_stacked],
      ]).get(chartType) || 0.3
      const initialGoogleOptions = {
        colors: orderedColors,

        // do not set width, height - the chart resizes itself
        // width: state.chartWidth(),
        // height: state.chartHeight(),
        lineWidth: chartType === "line" ? 2 : 1,
        title,
        fontSize: 11,
        hAxis: {
          //  title: "Time of Day",
          //  format:'HH:mm:ss',
          viewWindowMode: "maximized",
          slantedText: false,
          format: "HH:mm:ss",
          textStyle: {
            fontSize: 9,
          },
          gridlines: {
            color: "#EEE",
          },
        },
        vAxis: {
          title: unitsCurrent,
          viewWindowMode: (chartType === "area" || chartType === "stacked")
            ? "maximized"
            : "pretty",
          minValue: chartType === "stacked" ? undefined : -0.1,
          maxValue: chartType === "stacked" ? undefined : 0.1,
          direction: 1,
          textStyle: {
            fontSize: 9,
          },
          gridlines: {
            color: "#EEE",
          },
        },
        chartArea: {
          width: "65%",
          height: "80%",
        },
        focusTarget: "category",
        annotation: {
          1: {
            style: "line",
          },
        },
        pointsVisible: 0,
        titlePosition: "out",
        titleTextStyle: {
          fontSize: 11,
        },
        tooltip: {
          isHtml: false,
          ignoreBounds: true,
          textStyle: {
            fontSize: 9,
          },
        },
        curveType: "function",
        areaOpacity,
        isStacked: chartType === "stacked",
      }

      const googleInstance = ["area", "stacked"].includes(chartDetails.chart_type)
        ? new window.google.visualization.AreaChart(chartElement.current)
        : new window.google.visualization.LineChart(chartElement.current)

      googleInstance.draw(dataTable, initialGoogleOptions)

      googleOptions.current = initialGoogleOptions
      googleChartInstance.current = googleInstance
    }
  }, [attributes, chartData.result, chartDetails, chartElement, hasApiBeenLoaded, orderedColors,
    unitsCurrent])


  return (
    <div
      ref={chartElement}
      id={chartElementId}
      className={chartElementClassName}
    />
  )
}
