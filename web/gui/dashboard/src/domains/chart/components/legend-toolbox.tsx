import React from "react"

import { Icon } from "components/icon"

interface Props {

}
export const LegendToolbox = ({ // eslint-disable-line no-empty-pattern

}: Props) => (
  <div>
    <div className="netdata-legend-toolbox-button">
      <Icon iconType="left" />
    </div>
    <div className="netdata-legend-toolbox-button">
      <Icon iconType="reset" />
    </div>
    <div className="netdata-legend-toolbox-button">
      <Icon iconType="right" />
    </div>
    <div className="netdata-legend-toolbox-button">
      <Icon iconType="zoomIn" />
    </div>
    <div className="netdata-legend-toolbox-button">
      <Icon iconType="zoomOut" />
    </div>
  </div>
)
