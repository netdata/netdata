import React from 'react'
import Ps from 'perfect-scrollbar';

import './types/global'
import './dashboard'
import logo from './logo.svg'
import './App.css'

window.Ps = Ps

const App: React.FC = () => { // eslint-disable-line arrow-body-style
  return (
    <div className="App">
      <header className="App-header">
        <img src={logo} className="App-logo" alt="logo" />
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
