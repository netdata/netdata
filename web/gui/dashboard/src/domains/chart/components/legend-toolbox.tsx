import React, { useRef, useEffect } from "react"

import { Button } from "components/button"
import { Icon, IconType } from "components/icon"

type ClickCallback = (event: React.MouseEvent) => void
interface ToolboxButtonProps {
  iconType: IconType
  onClick: ClickCallback
  popoverContent: string
  popoverTitle: string
}
const ToolboxButton = ({
  iconType,
  onClick,
  popoverContent,
  popoverTitle,
}: ToolboxButtonProps) => {
  const buttonRef = useRef(null)
  useEffect(() => {
    if (buttonRef.current) {
      window.$(buttonRef.current).popover({
        container: "body",
        animation: false,
        html: true,
        trigger: "hover",
        placement: "bottom",
        delay: {
          show: window.NETDATA.options.current.show_help_delay_show_ms,
          hide: window.NETDATA.options.current.show_help_delay_hide_ms,
        },
        title: popoverTitle,
        content: popoverContent,
      })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Button
      className="netdata-legend-toolbox-button"
      onClick={onClick}
      ref={buttonRef}
    >
      <Icon iconType={iconType} />
    </Button>
  )
}

interface Props {
  onToolboxLeftClick: ClickCallback
  onToolboxResetClick: ClickCallback
  onToolboxRightClick: ClickCallback
  onToolboxZoomInClick: ClickCallback
  onToolboxZoomOutClick: ClickCallback
}
export const LegendToolbox = ({
  onToolboxLeftClick,
  onToolboxResetClick,
  onToolboxRightClick,
  onToolboxZoomInClick,
  onToolboxZoomOutClick,
}: Props) => (
  <div className="netdata-legend-toolbox">
    <ToolboxButton
      onClick={onToolboxLeftClick}
      iconType="left"
      popoverContent="Pan Left"
      popoverTitle="Pan the chart to the left. You can also <b>drag it</b> with your mouse or your
       finger (on touch devices).<br/><small>Help can be disabled from the settings.</small>"
    />
    <ToolboxButton
      onClick={onToolboxResetClick}
      iconType="reset"
      popoverContent="Chart Reset"
      popoverTitle="Reset all the charts to their default auto-refreshing state. You can also
       <b>double click</b> the chart contents with your mouse or your finger (on touch devices).
       <br/><small>Help can be disabled from the settings.</small>"
    />
    <ToolboxButton
      onClick={onToolboxRightClick}
      iconType="right"
      popoverContent="Pan Right"
      popoverTitle="Pan the chart to the right. You can also <b>drag it</b> with your mouse or your
       finger (on touch devices).<br/><small>Help, can be disabled from the settings.</small>"
    />
    <ToolboxButton
      onClick={onToolboxZoomInClick}
      iconType="zoomIn"
      popoverContent="Chart Zoom In"
      popoverTitle="Zoom in the chart. You can also press SHIFT and select an area of the chart, or
       press SHIFT or ALT and use the mouse wheel or 2-finger touchpad scroll to zoom in or out.
       <br/><small>Help, can be disabled from the settings.</small>"
    />
    <ToolboxButton
      onClick={onToolboxZoomOutClick}
      iconType="zoomOut"
      popoverContent="Chart Zoom Out"
      popoverTitle="Zoom out the chart. You can also press SHIFT or ALT and use the mouse wheel, or
       2-finger touchpad scroll to zoom in or out.<br/><small>Help, can be disabled from the
        settings.</small>"
    />
  </div>
)
