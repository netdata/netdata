import React from 'react'
import Ps from 'perfect-scrollbar'
import $ from 'jquery'

import './types/global'
import './App.css'

// with this syntax it loads asynchronously, after window. assignments are done
// @ts-ignore
import('./dashboard')
if (!window.netdataNoBootstrap) {
  // it needs to be imported indirectly, there's probably a bug in webpack
  import('dynamic-imports/bootstrap')
}

// support legacy code
window.Ps = Ps
window.$ = $
window.jQuery = window.$

const App: React.FC = () => { // eslint-disable-line arrow-body-style
  return (
    <div className="App">
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
