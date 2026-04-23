import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { ChakraProvider } from "@chakra-ui/react"
import { BrowserRouter } from "react-router-dom"
import App from "./App"
import theme from "./theme"
import { routerBasename } from "./config/runtime"
import { ToastContainer } from "./utils/toast"
import { PlatformProvider } from "./platform/PlatformContext"
import { platform as localPlatform } from "./platform/local"
import "./index.css"

if (typeof window !== "undefined") {
  document.addEventListener(
    "wheel",
    (e) => {
      if (e.ctrlKey) {
        e.preventDefault()
      }
    },
    { passive: false },
  )

  const preventGesture = (e: Event) => e.preventDefault()
  document.addEventListener("gesturestart", preventGesture, { passive: false })
  document.addEventListener("gesturechange", preventGesture, { passive: false })
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ChakraProvider theme={theme}>
      <PlatformProvider platform={localPlatform}>
        <BrowserRouter
          basename={routerBasename}
          future={{
            v7_startTransition: false,
            v7_relativeSplatPath: true,
          }}
        >
          <App />
        </BrowserRouter>
        <ToastContainer />
      </PlatformProvider>
    </ChakraProvider>
  </StrictMode>,
)
