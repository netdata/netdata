import React from "react"
import classNames from "classnames"

// todo add supoort for window.netdataIcons
type IconType = "left" | "reset" | "right" | "zoomIn" | "zoomOut" | "resize" | "lineChart"
  | "areaChart" | "noChart" | "loading" | "noData"
const typeToClassName = (iconType: IconType) => ({
  left: "fa-backward",
  reset: "fa-play",
  right: "fa-forward",
  zoomIn: "fa-plus",
  zoomOut: "fa-minus",
  resize: "fa-sort",
  lineChart: "fa-chart-line",
  areaChart: "fa-chart-area",
  noChart: "fa-chart-area",
  loading: "fa-sync-alt",
  noData: "fa-exclamation-triangle",
} as {[key in IconType]: string})[iconType]

interface Props {
  iconType: IconType
}
export const Icon = ({ iconType }: Props) => (
  <i className={classNames("fas", typeToClassName(iconType))} />
)
