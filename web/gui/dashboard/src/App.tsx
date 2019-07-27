import React, { useEffect } from "react"
import { useStore } from "react-redux"
import Ps from "perfect-scrollbar"
import $ from "jquery"

import { Portals } from "./components/Portals"
import "./types/global"
import "./App.css"

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
  useEffect(() => {
    dashboardModule.then((dashboard) => {
      // give working-dashboard module access to the store
      // (just for refractoring purposes)
      dashboard.startModule(store)
    })
  }, [store])
  return (
    <div className="App">
      <Portals />
      <header className="App-header">
        React app
        <a
          className="App-link"
          href="https://reactjs.org"
          target="_blank"
          rel="noopener noreferrer"
        >
          Learn React
        </a>
      </header>
    </div>
  )
}

export default App
