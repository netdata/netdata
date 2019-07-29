import React from "react"
import { createPortal } from "react-dom"
import { filter, mapObjIndexed, pipe } from "ramda"

import { ChartContainer } from "./chart-container"

const nodesArray = Array.from(document.querySelectorAll("[data-netdata-react]"))

const attributesMap = {
  id: "data-netdata-react",
  host: "data-host",
  title: "data-title",
  chartLibrary: "data-chart-library",
  width: "data-width",
  height: "data-height",
  after: "data-after",
  dygraphValueRange: "data-dygraph-valuerange",
}

const getAttributes = pipe(
  (node: Element) => mapObjIndexed(attribute => node.getAttribute(attribute), attributesMap),
  filter(x => !!x), // remove empty values
)

export const Portals = () => (
  <>
    {nodesArray.map((node, index) => {
      const attributesMapped = getAttributes(node) as { id: string }
      return (
        createPortal(
          <ChartContainer {...attributesMapped} uniqueId={`${attributesMapped.id}-${index}`} />,
          node,
        )
      )
    })}
  </>
)
