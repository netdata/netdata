import React, { useEffect, useState } from "react"
import { useSelector, useDispatch } from "react-redux"
import { useInterval } from "react-use"

import { AppStateT } from "store/app-state"

import { fetchDataAction } from "../actions"
import { Attributes } from "../utils/transformDataAttributes"
import { selectChartData, selectChartDetails } from "../selectors"

import { Chart } from "./chart"

export type Props = {
  attributes: Attributes
  chartUuid: string
  portalNode: HTMLElement
}

export const ChartContainer = ({
  attributes,
  chartUuid,
  portalNode,
}: Props) => {
  const chartDetails = useSelector((state: AppStateT) => selectChartDetails(
    state, { id: chartUuid },
  ))

  const [shouldFetch, setShouldFetch] = useState<boolean>(true)
  useInterval(() => {
    setShouldFetch(true)
  }, 2000)


  const chartData = useSelector((state: AppStateT) => selectChartData(state, { id: chartUuid }))
  const dispatch = useDispatch()
  useEffect(() => {
    if (shouldFetch && chartDetails) {
      setShouldFetch(false)
      dispatch(fetchDataAction.request({
        attributes,
        id: chartDetails.id,
        uuid: chartUuid,
      }))
    }
  }, [attributes, chartDetails, chartUuid, dispatch, shouldFetch])


  if (!chartData || !chartDetails) {
    return <span>loading...</span>
  }
  return (
    <Chart
      attributes={attributes}
      chartData={chartData}
      chartDetails={chartDetails}
      chartUuid={chartUuid}
      portalNode={portalNode}
    />
  )
}
