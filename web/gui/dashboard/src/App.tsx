import React from "react"
import Ps from "perfect-scrollbar"

import { loadCss } from "utils/css-loader"
import "domains/chart/utils/jquery-loader"
import { Portals } from "domains/chart/components/portals"
import "./types/global"

if (!window.netdataNoBootstrap) {
  // it needs to be imported indirectly, there's probably a bug in webpack
  import("dynamic-imports/bootstrap")
}

if (!window.netdataNoFontAwesome) {
  // @ts-ignore
  import("vendor/fontawesome-all-5.0.1.min")
}

// support legacy code
window.Ps = Ps

loadCss(window.NETDATA.themes.current.bootstrap_css)
loadCss(window.NETDATA.themes.current.dashboard_css)

const App: React.FC = () => { // eslint-disable-line arrow-body-style
  return (
    <div className="App">
      <Portals />
    </div>
  )
}

export default App
