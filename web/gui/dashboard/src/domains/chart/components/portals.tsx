import React from "react"
import { createPortal } from "react-dom"

import { getAttributes } from "../utils/transformDataAttributes"
import { ChartContainer } from "./chart-container"

const nodesArray = Array.from(document.querySelectorAll("[data-netdata-react]"))

export const Portals = () => (
  <>
    {nodesArray.map((node, index) => {
      const attributesMapped = getAttributes(node) as { id: string }
      return (
        createPortal(
          <ChartContainer
            attributes={getAttributes(node)}
            uniqueId={`${attributesMapped.id}-${index}`}
          />,
          node,
        )
      )
    })}
  </>
)
