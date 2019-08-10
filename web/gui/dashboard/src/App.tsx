import React, { useEffect, useState } from "react"
import { useStore } from "react-redux"
import Ps from "perfect-scrollbar"
import $ from "jquery"

import { Portals } from "./domains/chart/components/portals"
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
window.$ = $
window.jQuery = window.$

const App: React.FC = () => { // eslint-disable-line arrow-body-style
  const store = useStore()
  const [hasStarted, setHasStarted] = useState(false)
  useEffect(() => {
    dashboardModule.then((dashboard) => {
      // give working-dashboard module access to the store
      // (just for refractoring purposes)
      dashboard.startModule(store)
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
