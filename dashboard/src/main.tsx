import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

// Fonts are self-hosted: a privacy-first product must not call a font CDN.
import '@fontsource-variable/geist/wght.css'
import '@fontsource-variable/geist-mono/wght.css'

import './styles/tokens.css'
import './styles/base.css'
import App from './App.tsx'
import { AuthProvider } from './lib/auth'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider>
      <App />
    </AuthProvider>
  </StrictMode>,
)
