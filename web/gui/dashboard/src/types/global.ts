/* eslint-disable camelcase */
// @ts-ignore isolated-modules
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
  serverDefault: string
  start: () => void
}

type jQuery = any

interface Window {
  NETDATA: NETDATA
  Ps: any // perfect scrollbar
  $: jQuery
  jQuery: jQuery

  // user configuration options
  netdataNoBootstrap: boolean | undefined

  __REDUX_DEVTOOLS_EXTENSION__: (() => void | undefined)
}
