import React from "react"
import classNames from "classnames"

import { seconds4human } from "../utils/seconds4human"
import { ChartData, ChartDetails } from "../chart-types"

interface Props {
  chartData: ChartData
  chartDetails: ChartDetails
  chartLibrary: string
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
  // todo if there's no data (maybe there wont be situation like this), then use "collected"
  const viewed = chartData.view_update_every
  if (collected === viewed) {
    return `resolution ${seconds4human(collected)}`
  }

  return `resolution ${seconds4human(viewed)}, collected every ${seconds4human(collected)}`
}

export const ChartLegend = ({
  chartData,
  chartDetails,
  chartLibrary,
}: Props) => {
  const netdataLast = chartData.last_entry * 1000
  // todo lift before/after to the state (when doing highlighting/pan/zoom)
  // when requested_padding, view_before is not always equal this.data_before
  const viewBefore = chartData.before * 1000
  const dataUpdateEvery = chartData.view_update_every * 1000

  // eslint-disable-next-line no-unused-vars,@typescript-eslint/no-unused-vars
  const showUndefined = Math.abs(netdataLast - viewBefore) > dataUpdateEvery
  // todo make separate case for showUndefined

  const legendDate = new Date(viewBefore)
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
      {/* title_units */}
      <br />
      {/* perfect_scroller */}
      {/* content */}
    </div>
  )
}
