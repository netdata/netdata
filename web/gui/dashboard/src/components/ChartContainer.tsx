import React from "react"

export type Props = {
  id?: string
  host?: string
  title?: string
  chartLibrary?: string
  width?: string
  height?: string
  after?: string
  dygraphValueRange?: string
}

export const ChartContainer = (props: Props) => { // eslint-disable-line
  return (
    <div>Chart Container</div>
  )
}
