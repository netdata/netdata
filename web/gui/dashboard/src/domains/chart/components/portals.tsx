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
            // todo change to uuid generator (when we disconnect dashboard.js)
            chartUuid={`${attributesMapped.id}-${index}`}
            portalNode={(node as HTMLElement)}
          />,
          node,
        )
      )
    })}
  </>
)
