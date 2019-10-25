import React, { useCallback, useState, useEffect } from "react"

import { ToolboxButton } from "domains/chart/components/toolbox-button"
import { setResizeHeightAction } from "domains/chart/actions"
import { useDispatch } from "react-redux"

interface Props {
  chartContainerElement: HTMLElement
  chartUuid: string
}
// eslint-disable-next-line no-empty-pattern
export const ResizeHandler = ({
  chartContainerElement,
  chartUuid,
}: Props) => {
  const [resizing, setResizing] = useState<
    { mouseStartY: number, startHeight: number }
  >()

  const dispatch = useDispatch()

  // handle resizing effect
  useEffect(() => { // eslint-disable-line consistent-return
    if (resizing) {
      const handleMove = (event: MouseEvent | TouchEvent) => {
        const y = event.type === "mousemove"
          ? (event as MouseEvent).clientY
          // "touchmove"
          // @ts-ignore
          : (event as TouchEvent).touches.item(event.touches - 1).pageY

        const newHeight = resizing.startHeight + y - resizing.mouseStartY

        if (newHeight >= 70) {
          // eslint-disable-next-line no-param-reassign
          chartContainerElement.style.height = `${newHeight.toString()}px`
          // todo when attributes.id are present, hook height to localStorage
          dispatch(setResizeHeightAction({
            id: chartUuid,
            resizeHeight: newHeight,
          }))
        }
      }
      document.addEventListener("mousemove", handleMove, false)
      document.addEventListener("touchmove", handleMove, false)
      // on exit, remove listeners
      return () => {
        document.removeEventListener("mousemove", handleMove)
        document.removeEventListener("touchmove", handleMove)
      }
    }
  }, [chartContainerElement, chartUuid, dispatch, resizing])

  // process end event
  useEffect(() => { // eslint-disable-line consistent-return
    if (resizing) {
      const handleEndEvent = () => {
        setResizing(undefined)
      }
      document.addEventListener("mouseup", handleEndEvent, false)
      document.addEventListener("touchend", handleEndEvent, false)
      return () => {
        document.removeEventListener("mouseup", handleEndEvent)
        document.removeEventListener("touchend", handleEndEvent)
      }
    }
  }, [resizing])

  // start resizing handler
  const resizeStartHandler = useCallback((event) => {
    event.preventDefault()
    setResizing({
      // todo handle touch event
      mouseStartY: event.clientY,
      startHeight: chartContainerElement.clientHeight,
    })
  }, [chartContainerElement])

  return (
    <ToolboxButton
      className="netdata-legend-resize-handler"
      onDoubleClick={(event: React.MouseEvent) => {
        event.preventDefault()
        event.stopPropagation()
      }}
      onMouseDown={resizeStartHandler}
      onTouchStart={resizeStartHandler}
      iconType="resize"
      popoverContent="Chart Resize"
      popoverTitle="Drag this point with your mouse or your finger (on touch devices), to resize
       the chart vertically. You can also <b>double click it</b> or <b>double tap it</b> to reset
        between 2 states: the default and the one that fits all the values.<br/><small>Help,
         can be disabled from the settings.</small>"
    />
  )
}
