import React, { useEffect, useState } from "react"
import { useStore } from "react-redux"
import Ps from "perfect-scrollbar"

import { loadCss } from "utils/css-loader"
import "domains/chart/utils/jquery-loader"
import { Portals } from "domains/chart/components/portals"
import "./types/global"

// with this syntax it loads asynchronously, after window. assignments are done
// @ts-ignore
const dashboardModule = import("./dashboard")
if (!window.netdataNoBootstrap) {
  // it needs to be imported indirectly, there's probably a bug in webpack
  import("dynamic-imports/bootstrap")
}

// support legacy code
window.Ps = Ps

loadCss(window.NETDATA.themes.current.bootstrap_css)
loadCss(window.NETDATA.themes.current.dashboard_css)

const App: React.FC = () => { // eslint-disable-line arrow-body-style
  const store = useStore()
  const [hasStarted, setHasStarted] = useState(false)
  useEffect(() => {
    dashboardModule.then((dashboard) => {
      // give working-dashboard module access to the store
      // (just for refractoring purposes)
      dashboard.startModule(store)

      // for use by main.js
      // we cannot make main.js a module yet, because index.html uses window onclick handlers
      // like onclick="saveSnapshotSetCompression('none'); return false;"
      window.reduxStore = store

      setHasStarted(true)
    })
  }, [store])
  if (!hasStarted) {
    return (
      <div>loading...</div>
    )
  }
  return (
    <div className="App">
      <Portals />
    </div>
  )
}

export default App
