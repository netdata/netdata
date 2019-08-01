import React from "react"
import classNames from "classnames"

import { ChartDetails } from "../chart-types"

interface Props {
  chartDetails: ChartDetails
  chartLibrary: string
}
export const ChartLegend = ({
  chartDetails, // eslint-disable-line no-unused-vars
  chartLibrary,
}: Props) => (
  <div className={classNames(
    "netdata-chart-legend",
    `netdata-${chartLibrary}-legend`,
  )}
  >
    {/* title_date */}
    {/* title_time */}
    {/* title_units */}
    {/* perfect_scroller */}
    {/* content */}


    {/* title_date */}
    {/* title_date */}
    {/* title_date */}
  </div>
)

/*

 */
