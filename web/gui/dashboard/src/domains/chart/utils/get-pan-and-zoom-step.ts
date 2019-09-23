type GetPanAndZoomStep = (event: React.MouseEvent) => number
export const getPanAndZoomStep: GetPanAndZoomStep = (event) => {
  if (event.ctrlKey) {
    return window.NETDATA.options.current.pan_and_zoom_factor
      * window.NETDATA.options.current.pan_and_zoom_factor_multiplier_control
  } if (event.shiftKey) {
    return window.NETDATA.options.current.pan_and_zoom_factor
      * window.NETDATA.options.current.pan_and_zoom_factor_multiplier_shift
  } if (event.altKey) {
    return window.NETDATA.options.current.pan_and_zoom_factor
      * window.NETDATA.options.current.pan_and_zoom_factor_multiplier_alt
  }
  return window.NETDATA.options.current.pan_and_zoom_factor
}
