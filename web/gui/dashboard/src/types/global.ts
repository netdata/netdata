/* eslint-disable camelcase */
// @ts-ignore isolated-modules

interface Theme {
  background: string
  foreground: string
  grid: string
  axis: string
  highlight: string
  colors: string[]
  d3pie: {
    [d3pieKey: string]: string
  }
  easypiechart_track: string
  easypiechart_scale: string
  gauge_pointer: string
  gauge_stroke: string
  gauge_gradient: string
}

interface NETDATA {
  chartDefaults: {
    width: string | null
    height: string | null
    min_width: string | null
    library: string
    method: string
    before: number
    after: number
    pixels_per_point: number
    fill_luminance: number
  }
  colorHex2Rgb: (hex: string) => { r: string, g: string, b: string }
  options: {
    current: {
      units: string
      temperature: string
      seconds_as_time: boolean
      timezone: string | undefined
      user_set_server_timezone: string

      legend_toolbox: boolean
      resize_charts: boolean

      pixels_per_point: number

      idle_between_charts: number

      fast_render_timeframe: number
      idle_between_loops: number
      idle_parallel_loops: number
      idle_lost_focus: number

      global_pan_sync_time: number

      sync_selection_delay: number
      sync_selection: boolean

      pan_and_zoom_delay: number
      sync_pan_and_zoom: boolean
      pan_and_zoom_data_padding: boolean

      update_only_visible: boolean

      parallel_refresher: boolean

      concurrent_refreshes: boolean

      destroy_on_hide: boolean
      show_help: boolean
      show_help_delay_show_ms: number
      show_help_delay_hide_ms: number

      eliminate_zero_dimensions: boolean

      stop_updates_when_focus_is_lost: boolean
      stop_updates_while_resizing: number

      double_click_speed: number

      smooth_plot: boolean

      color_fill_opacity_line: number
      color_fill_opacity_area: number
      color_fill_opacity_stacked: number

      pan_and_zoom_factor: number
      pan_and_zoom_factor_multiplier_control: number
      pan_and_zoom_factor_multiplier_shift: number
      pan_and_zoom_factor_multiplier_alt: number

      abort_ajax_on_scroll: boolean
      async_on_scroll: boolean
      onscroll_worker_duration_threshold: number

      retries_on_data_failures: number
    }
  }
  dateTime: {
    localeDateString: (d: Date) => string
    localeTimeString: (d: Date) => string
  }
  serverDefault: string
  start: () => void
  themes: { [key: string]: Theme }
}

type jQuery = any

interface Window {
  NETDATA: NETDATA
  Ps: any // perfect scrollbar
  $: jQuery
  jQuery: jQuery
  smoothPlotter: () => void

  // user configuration options
  netdataNoBootstrap: boolean | undefined

  __REDUX_DEVTOOLS_EXTENSION__: (() => void | undefined)
}
