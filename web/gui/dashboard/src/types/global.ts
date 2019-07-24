// @ts-ignore isolated-modules
interface NETDATA {
  start: () => void
}

type jQuery = any

interface Window {
  NETDATA: NETDATA
  Ps: any // perfect scrollbar
  $: jQuery
  jQuery: jQuery
}
