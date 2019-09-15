import { Attributes } from "./transformDataAttributes"
import { ChartLibraryConfig } from "./chartLibrariesSettings"

type GetChartPixelsPerPoint = (arg: {
  attributes: Attributes,
  chartSettings: ChartLibraryConfig,
}) => number

export const getChartPixelsPerPoint: GetChartPixelsPerPoint = ({
  attributes, chartSettings,
}) => {
  const {
    pixelsPerPoint: pixelsPerPointAttribute = 1,
  } = attributes
  const pixelsPerPointSetting = chartSettings.pixelsPerPoint(attributes)

  return Math.max(...[
    pixelsPerPointAttribute,
    pixelsPerPointSetting,
    window.NETDATA.options.current.pixels_per_point,
  ].filter((px) => typeof px === "number"))
}
