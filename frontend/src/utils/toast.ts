import { createStandaloneToast, UseToastOptions, ToastId } from '@chakra-ui/react'
import theme from '../theme'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const { toast: chakraToast, ToastContainer }: any = createStandaloneToast({
  theme,
  defaultOptions: {
    status: 'error',
    duration: 5000,
    isClosable: true,
    position: 'bottom-right',
  },
})

let errorToastId: ToastId | null = null
let errorCount = 0
const errorSummaries = new Set<string>()

/**
 * Extracts a short error code or summary from a ConnectRPC error message.
 * ConnectRPC messages often look like "[unavailable] upstream request timeout"
 * or REST responses like "HTTP 500 Internal Server Error"
 */
function getErrorSummary(options: UseToastOptions): string {
  const text = (options.description || options.title || 'Unknown error').toString()
  
  // ConnectRPC: matches "[code] message"
  const connectMatch = text.match(/^\[(\w+)\]/)
  if (connectMatch) return connectMatch[1]
  
  // HTTP status: "HTTP 500"
  const httpMatch = text.match(/HTTP \d+/)
  if (httpMatch) return httpMatch[0]
  
  // Generic fallback: first part of message (before colon or newline)
  return text.split(':')[0].split('\n')[0].substring(0, 32).trim()
}

/**
 * Global toast utility that intercepts 'error' status toasts to combine multiple
 * backend errors into a single notification with a counter.
 */
interface CustomToast {
  (options: UseToastOptions): ToastId | undefined;
  close: typeof chakraToast.close;
  closeAll: typeof chakraToast.closeAll;
  update: typeof chakraToast.update;
  isActive: typeof chakraToast.isActive;
}

const toast: CustomToast = (options: UseToastOptions) => {
  const status = options.status || 'error'

  if (status === 'error') {
    // Silence error toasts if we are on a demo route
    const isDemoRoute = window.location.pathname.includes('/demo') || 
                        window.location.pathname.includes('/app/demo');

    if (isDemoRoute) {
      return undefined
    }

    const summary = getErrorSummary(options)
    
    // Check if an error toast is already active
    if (errorToastId && chakraToast.isActive(errorToastId)) {
      errorCount++
      errorSummaries.add(summary)
      
      chakraToast.update(errorToastId, {
        ...options,
        title: `${errorCount} Requests Failed`,
        description: `Errors: ${Array.from(errorSummaries).join(', ')}`,
        duration: 5000, // Refresh the timer
      })
      return errorToastId;
    } else {
      // First error or previous toast was closed
      errorCount = 1
      errorSummaries.clear()
      errorSummaries.add(summary)
      
      const originalOnCloseComplete = options.onCloseComplete
      
      errorToastId = chakraToast({
        ...options,
        title: options.title || 'Request Failed',
        onCloseComplete: () => {
          if (originalOnCloseComplete) originalOnCloseComplete()
          errorToastId = null
          errorCount = 0
          errorSummaries.clear()
        }
      })
      return errorToastId;
    }
  }

  return chakraToast(options)
}

// Proxy standard chakra toast methods for complete compatibility
toast.close = chakraToast.close
toast.closeAll = chakraToast.closeAll
toast.update = chakraToast.update
toast.isActive = chakraToast.isActive

export { toast, ToastContainer }
