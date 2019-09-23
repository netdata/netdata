import React from "react"

import { Button } from "components/button"
import { Icon, IconType } from "components/icon"

type ClickCallback = (event: React.MouseEvent) => void
interface ToolboxButtonProps {
  iconType: IconType
  onClick: ClickCallback
}
const ToolboxButton = ({ iconType, onClick }: ToolboxButtonProps) => (
  <Button
    className="netdata-legend-toolbox-button"
    onClick={onClick}
  >
    <Icon iconType={iconType} />
  </Button>
)

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
    <ToolboxButton onClick={onToolboxLeftClick} iconType="left" />
    <ToolboxButton onClick={onToolboxResetClick} iconType="reset" />
    <ToolboxButton onClick={onToolboxRightClick} iconType="right" />
    <ToolboxButton onClick={onToolboxZoomInClick} iconType="zoomIn" />
    <ToolboxButton onClick={onToolboxZoomOutClick} iconType="zoomOut" />
  </div>
)
