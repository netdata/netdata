import React, { Fragment } from "react"
import classNames from "classnames"

import { find } from "ramda"
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
  hoveredX: number | null
  legendFormatValue: (value: number | string) => (number | string)
  selectedDimensions: string[]
  setSelectedDimensions: (selectedDimensions: string[]) => void
  unitsCurrent: string
  viewBefore: number
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

type GetNewSelectedDimensions = (arg: {
  allDimensions: string[],
  selectedDimensions: string[],
  clickedDimensionName: string,
  isModifierKeyPressed: boolean,
}) => string[]

export const getNewSelectedDimensions: GetNewSelectedDimensions = ({
  allDimensions,
  selectedDimensions,
  clickedDimensionName,
  isModifierKeyPressed,
}) => {
  // when selectedDimensions is empty, then all dimensions should be enabled
  // let's narrow this case now
  const enabledDimensions = selectedDimensions.length === 0 ? allDimensions : selectedDimensions
  const isCurrentlySelected = enabledDimensions.includes(clickedDimensionName)

  let newSelectedDimensions: string[]
  if (!isModifierKeyPressed
    && ((isCurrentlySelected && enabledDimensions.length > 1) || !isCurrentlySelected)
  ) {
    newSelectedDimensions = [clickedDimensionName]
  } else if (isCurrentlySelected) { // modifier key pressed
    newSelectedDimensions = enabledDimensions.filter(
      (dimension) => dimension !== clickedDimensionName,
    )
  } else { // modifier key pressed
    newSelectedDimensions = enabledDimensions.concat(clickedDimensionName)
  }

  if (newSelectedDimensions.length === allDimensions.length) {
    return []
  }
  return newSelectedDimensions
}

export const ChartLegend = ({
  // attributes,
  chartData,
  chartDetails,
  chartLibrary,
  colors,
  hoveredX,
  legendFormatValue,
  selectedDimensions,
  setSelectedDimensions,
  unitsCurrent,
  viewBefore,
}: Props) => {
  const netdataLast = chartData.last_entry * 1000
  const dataUpdateEvery = chartData.view_update_every * 1000

  const showUndefined = Math.abs(netdataLast - viewBefore) > dataUpdateEvery
  // eslint-disable-next-line no-void
  void (showUndefined)
  // todo make separate case for showUndefined

  const legendDate = new Date(hoveredX || viewBefore)

  // todo make a possibility to add chartLegened when there's not chartData
  // (if this situation is possible)

  // @ts-ignore ignoring because options.current has inconsistent structure
  const colorFillOpacity = window.NETDATA.options.current[
    `color_fill_opacity_${chartDetails.chart_type}`
  ]

  const handleDimensionClick = (clickedDimensionName: string) => (event: React.MouseEvent) => {
    event.preventDefault()
    const isModifierKeyPressed = event.shiftKey || event.ctrlKey
    const newSelectedDimensions = getNewSelectedDimensions({
      allDimensions: chartData.dimension_names,
      selectedDimensions,
      clickedDimensionName,
      isModifierKeyPressed,
    })
    setSelectedDimensions(newSelectedDimensions)
  }

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
      <span className="netdata-legend-title-units">{unitsCurrent}</span>
      <br />
      {/* perfect_scroller */}
      <div className="netdata-legend-series-content">
        {chartData.dimension_names.map((dimensionName, i) => {
          // todo dimension could be a separate component
          const color = colors[dimensionName]
          const rgb = window.NETDATA.colorHex2Rgb(color)

          const isSelected = selectedDimensions.length === 0
            || selectedDimensions.includes(dimensionName)

          let value
          if (hoveredX) {
            const hoveredValueArray = find(
              (x: any) => x[0] === hoveredX,
              chartData.result.data,
            ) as number[]
            if (!hoveredValueArray) {
              value = ""
            } else {
              value = hoveredValueArray[i + 1]
            }
          } else {
            value = chartData.view_latest_values[i]
          }

          return (
            <Fragment key={dimensionName}>
              {i !== 0 && <br />}
              {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events */}
              <span
                title={dimensionName}
                className={classNames(
                  "netdata-legend-name",
                  isSelected ? "selected" : "not-selected",
                )}
                onClick={handleDimensionClick(dimensionName)}
                role="button"
                style={{ color }}
                tabIndex={0}
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
              {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events */}
              <span
                title={dimensionName}
                className={classNames(
                  "netdata-legend-value",
                  !isSelected && "hidden",
                )}
                onClick={handleDimensionClick(dimensionName)}
                role="button"
                style={{ color }} // omitted !important during refractor (react doesn't support it)
                tabIndex={0}
              >
                {legendFormatValue(
                  value,
                )}
              </span>
            </Fragment>
          )
        })}
      </div>
    </div>
  )
}
