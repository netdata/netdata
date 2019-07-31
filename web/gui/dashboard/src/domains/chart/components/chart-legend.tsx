import React from "react"
import classNames from "classnames"

interface Props {
  chartLibrary: string
}
export const ChartLegend = ({
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
