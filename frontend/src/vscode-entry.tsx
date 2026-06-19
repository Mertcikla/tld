import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ChakraProvider } from '@chakra-ui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import App from './App'
import theme from './theme'
import { ToastContainer } from './utils/toast'
import { PlatformProvider } from './platform/PlatformContext'
import { platform as localPlatform } from './platform/local'
import './index.css'

declare global {
  interface Window {
    __TLD_DIAGRAM_ID__?: number
    __TLD_VSCODE__?: boolean
    __TLD_VSCODE_API__?: {
      postMessage: (msg: unknown) => void
    }
    __TLD_SERVER_URL__?: string
  }
}

const diagramId = window.__TLD_DIAGRAM_ID__
const initialPath = diagramId != null ? `/views/${diagramId}` : '/views'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: false,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ChakraProvider theme={theme}>
        <PlatformProvider platform={localPlatform}>
          <MemoryRouter initialEntries={[initialPath]}>
            <App />
          </MemoryRouter>
          <ToastContainer />
        </PlatformProvider>
      </ChakraProvider>
    </QueryClientProvider>
  </StrictMode>,
)
