import React, { Fragment } from "react"
import classNames from "classnames"

import { seconds4human } from "../utils/seconds4human"
import { Attributes } from "../utils/transformDataAttributes"
import { ChartData, ChartDetails } from "../chart-types"

interface Props {
  attributes: Attributes
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: string
  colors: {
    [key: string]: string
  }
  units: string
}

export const legendPluginModuleString = (withContext: boolean, chartDetails: ChartDetails) => {
  let str = " "
  let context = ""

  if (withContext && typeof chartDetails.context === "string") {
    // eslint-disable-next-line prefer-destructuring
    context = chartDetails.context
  }

  if (typeof chartDetails.plugin === "string" && chartDetails.plugin !== "") {
    str = chartDetails.plugin

    if (str.endsWith(".plugin")) {
      str = str.substring(0, str.length - 7)
    }

    if (typeof chartDetails.module === "string" && chartDetails.module !== "") {
      str += `:${chartDetails.module}`
    }

    if (withContext && context !== "") {
      str += `, ${context}`
    }
  } else if (withContext && context !== "") {
    str = context
  }
  return str
}

const legendResolutionTooltip = (chartData: ChartData, chartDetails: ChartDetails) => {
  const collected = chartDetails.update_every
  // todo if there's no data (but maybe there wont be situation like this), then use "collected"
  const viewed = chartData.view_update_every
  if (collected === viewed) {
    return `resolution ${seconds4human(collected)}`
  }

  return `resolution ${seconds4human(viewed)}, collected every ${seconds4human(collected)}`
}

export const ChartLegend = ({
  // attributes,
  chartData,
  chartDetails,
  chartLibrary,
  colors,
  units,
}: Props) => {
  const netdataLast = chartData.last_entry * 1000
  // todo lift before/after to the state (when doing highlighting/pan/zoom)
  // when requested_padding, view_before is not always equal this.data_before
  const viewBefore = chartData.before * 1000
  const dataUpdateEvery = chartData.view_update_every * 1000

  const showUndefined = Math.abs(netdataLast - viewBefore) > dataUpdateEvery
  console.log("showUndefined", showUndefined) // eslint-disable-line no-console
  // todo make separate case for showUndefined

  const legendDate = new Date(viewBefore)

  // todo make a possibility to add chartLegened when there's not chartData
  // (if this situation is possible)

  // @ts-ignore ignoring because options.current has inconsistent structure
  const colorFillOpacity = window.NETDATA.options.current[
    `color_fill_opacity_${chartDetails.chart_type}`
  ]
  return (
    <div className={classNames(
      "netdata-chart-legend",
      `netdata-${chartLibrary}-legend`,
    )}
    >
      <span
        className="netdata-legend-title-date"
        title={legendPluginModuleString(true, chartDetails)}
      >
        {window.NETDATA.dateTime.localeDateString(legendDate)}
      </span>
      <br />
      <span
        className="netdata-legend-title-time"
        title={legendResolutionTooltip(chartData, chartDetails)}
      >
        {window.NETDATA.dateTime.localeTimeString(legendDate)}
      </span>
      <br />
      <span className="netdata-legend-title-units">{units}</span>
      <br />
      {/* perfect_scroller */}
      <div className="netdata-legend-series-content">
        {chartData.dimension_names.map((dimensionName, i) => {
          const color = colors[dimensionName]
          const rgb = window.NETDATA.colorHex2Rgb(color)
          const isSelected = true // todo lift to state
          return (
            <Fragment key={dimensionName}>
              {i !== 0 && <br />}
              <span
                title={dimensionName}
                className={classNames(
                  "netdata-legend-name",
                  { "netdata-legend-name": isSelected },
                )}
                style={{ color }} // omitted !important during refractor (react doesn't support it)
              >
                <table
                  className={`netdata-legend-name-table-${chartDetails.chart_type}`}
                  style={{
                    backgroundColor: `rgba(${rgb.r},${rgb.g},${rgb.b},${colorFillOpacity})`,
                  }}
                >
                  <tbody>
                    <tr className="netdata-legend-name-tr">
                      <td className="netdata-legend-name-td" />
                    </tr>
                  </tbody>
                </table>
                {" "}
                {dimensionName}
              </span>
              <span
                title={dimensionName}
                className={classNames(
                  "netdata-legend-value",
                  { selected: isSelected },
                )}
                style={{ color }} // omitted !important during refractor (react doesn't support it)
                // value was set in dashboard.js by this.legendSetLabelValue
                // this.legendSetLabelValue could be called by: legendFormatter (dygraph internal
                // function), legendShowUndefined (clearing), or this.legendShowLatestValues
                // todo this needs to be further formatted using this.legendFormatValue()
                // (but first, charts needs to be configured)
              >
                {chartData.view_latest_values[i]}
              </span>
            </Fragment>
          )
        })}
      </div>
      {/* content */}
    </div>
  )
}
