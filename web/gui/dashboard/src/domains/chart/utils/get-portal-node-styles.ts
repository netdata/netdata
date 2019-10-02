import { Attributes } from "./transformDataAttributes"
import { ChartLibraryConfig } from "./chartLibrariesSettings"

type GetPortalNodeStyles = (attributes: Attributes, chartSettings: ChartLibraryConfig) => {
  height: string | undefined,
  width: string | undefined,
  minWidth: string | undefined
}
export const getPortalNodeStyles: GetPortalNodeStyles = (attributes, chartSettings) => {
  let width
  if (typeof attributes.width === "string") {
    // eslint-disable-next-line prefer-destructuring
    width = attributes.width
  } else if (typeof attributes.width === "number") {
    width = `${attributes.width.toString()}px`
  }
  let height
  if (chartSettings.aspectRatio === undefined) {
    if (typeof attributes.height === "string") {
      // eslint-disable-next-line prefer-destructuring
      height = attributes.height
    } else if (typeof attributes.height === "number") {
      height = `${attributes.height.toString()}px`
    }
  }
  const chartDefaultsMinWidth = window.NETDATA.chartDefaults.min_width
  const minWidth = chartDefaultsMinWidth === null
    ? undefined
    : chartDefaultsMinWidth
  return {
    height,
    width,
    minWidth,
  }
}
