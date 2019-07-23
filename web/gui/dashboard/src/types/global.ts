// @ts-ignore isolated-modules
interface NETDATA {
  start: () => void
}

interface Window {
  NETDATA: NETDATA
  Ps: any // perfect scrollbar
}
