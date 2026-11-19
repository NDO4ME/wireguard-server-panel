import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './theme/app.css'
import './theme/extra.css'
import './theme/ndo4.css'
import { initPointerLight } from './theme/pointer-light'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)

initPointerLight()
