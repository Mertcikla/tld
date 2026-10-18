import { extendTheme, type ThemeConfig } from '@chakra-ui/react'
import { floatingSurfaceStyles } from './styles/surfaces'

const config: ThemeConfig = {
  initialColorMode: 'dark',
  useSystemColorMode: false,
}

export default extendTheme({
  config,
  fonts: {
    heading: `'Josefin Sans', system-ui, -apple-system, sans-serif`,
    body: `'Metrophobic', system-ui, -apple-system, sans-serif`,
  },
  breakpoints: {
    base: '0em',
    sm: '30em',
    md: '48em',
    compact: '53em',
    lg: '62em',
    xl: '80em',
    '2xl': '96em',
  },
  colors: {
    clay: {
      bg: 'var(--bg-element)',
      out: 'color-mix(in srgb, var(--bg-element), black 10%)',
      in: 'color-mix(in srgb, var(--bg-element), black 10%)',
      text: '#cbd5e0',
      dim: '#4a5568',
    },
    glass: {
      bg: 'var(--bg-panel)',
      border: 'var(--border-main)',
    },
  },
  semanticTokens: {
    colors: {
      text: {
        secondary: { default: 'gray.300' },
        muted: { default: 'gray.400' },
      },
    },
  },
  shadows: {
    // Semantic shadows for glass panels
    'panel': '0 8px 32px rgba(0,0,0,0.5)',
    'panel-sm': '0 4px 10px rgba(0,0,0,0.3)',
    'panel-hover': '0 6px 15px rgba(0,0,0,0.4)',
    // Keep legacy variants and downstream themes using these names functional.
    'clay-out': 'var(--chakra-shadows-panel-sm)',
    'clay-out-hover': 'var(--chakra-shadows-panel-hover)',
    'clay-in': 'inset 0 1px 3px rgba(0,0,0,0.3)',
    'node-selected': '0 0 0 3px rgba(99,179,237,0.35), 0 10px 36px rgba(0,0,0,0.55), 0 3px 10px rgba(0,0,0,0.4)',
    'node-source': '0 0 0 3px rgba(72, 151, 187, 0.55), 0 0 24px rgba(72, 149, 187, 0.25)',
  },
  transitions: {
    easing: {
      pop: 'cubic-bezier(0.175, 0.885, 0.32, 1.275)',
    },
    duration: {
      fast: '0.15s',
      normal: '0.2s',
      slow: '0.3s',
    },
  },
  styles: {
    global: {
      body: { bg: 'var(--bg-main)', color: 'gray.100' },
      '.glass': {
        bg: 'glass.bg',
        backdropFilter: 'blur(20px)',
        border: '1px solid',
        borderColor: 'glass.border',
      },
      // Hide popper/popover tooltip arrows rendered by Popper/Chakra
      '[data-popper-arrow], [data-popper-arrow-inner], .chakra-popover__arrow, .chakra-tooltip__arrow': {
        display: 'none !important',
        width: '0 !important',
        height: '0 !important',
      },
    },
  },
  components: {
    Popover: {
      baseStyle: {
        content: {
          ...floatingSurfaceStyles,
          bg: 'var(--bg-menu)',
          _focus: { boxShadow: 'panel' },
        },
        arrow: {
          display: 'none',
        },
      },
    },
    Modal: {
      baseStyle: {
        overlay: { bg: 'blackAlpha.700', backdropFilter: 'blur(4px)' },
        dialog: {
          ...floatingSurfaceStyles,
        },
        header: {
          color: 'gray.100',
          fontSize: 'lg',
          fontWeight: 'bold',
          borderBottom: '1px solid var(--border-main)',
        },
        body: {
          color: 'gray.300',
        },
        footer: {
          borderTop: '1px solid var(--border-main)',
        },
      },
    },
    Drawer: {
      baseStyle: {
        dialog: {
          bg: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border-main)',
          boxShadow: 'panel',
        },
      },
    },
    Menu: {
      baseStyle: {
        list: {
          ...floatingSurfaceStyles,
          bg: 'var(--bg-menu)',
          py: 2,
          minW: '200px',
          zIndex: 9999,
        },
        item: {
          bg: 'transparent',
          color: 'clay.text',
          fontSize: 'sm',
          _hover: { bg: 'whiteAlpha.100' },
          _focus: { bg: 'whiteAlpha.100' },
          _active: { bg: 'whiteAlpha.200' },
        },
      },
    },
    Tooltip: {
      baseStyle: {
        bg: 'var(--bg-menu)',
        color: 'clay.text',
        fontSize: '12px',
        borderRadius: '8px',
        px: 3,
        py: 1.5,
        boxShadow: 'panel-sm',
        border: '1px solid var(--border-main)',
        zIndex: 99999,
        arrow: {
          display: 'none',
        },
      },
    },
    Select: {
      variants: {
        clay: {
          field: {
            bg: 'clay.out',
            color: 'clay.text',
            boxShadow: 'clay-out',
            borderRadius: '8px',
            border: '1px solid rgba(255,255,255,0.06)',
            transition: 'all 0.2s cubic-bezier(0.175, 0.885, 0.32, 1.275)',
            _hover: { bg: 'color-mix(in srgb, var(--bg-element), white 15%)' },
            _active: { bg: 'clay.in', boxShadow: 'clay-in' },
            _focus: { ring: '1px', ringColor: 'var(--accent)' },
            'option': {
              bg: 'clay.bg',
              color: 'clay.text',
            }
          },
          icon: {
            color: 'clay.text',
          },
        },
        elevated: {
          field: {
            bg: 'whiteAlpha.100',
            color: 'whiteAlpha.900',
            borderRadius: 'md',
            border: '1px solid',
            borderColor: 'whiteAlpha.100',
            boxShadow: '0 4px 10px rgba(0,0,0,0.3)',
            transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
            _hover: { bg: 'whiteAlpha.200', transform: 'translateY(-1px)', boxShadow: 'panel-hover' },
            _active: { transform: 'translateY(0)' },
            'option': {
              bg: 'gray.800',
              color: 'white',
            }
          },
          icon: {
            color: 'whiteAlpha.600',
          },
        },
      },
      defaultProps: {
        variant: 'elevated',
      },
    },
    Input: {
      baseStyle: {
        field: { _placeholder: { color: 'text.muted', opacity: 1 } },
      },
      variants: {
        clay: {
          field: {
            bg: 'gray.800',
            color: 'clay.text',
            borderRadius: '8px',
            border: '1px solid rgba(255,255,255,0.08)',
            transition: 'all 0.2s cubic-bezier(0.175, 0.885, 0.32, 1.275)',
            _hover: { borderColor: 'whiteAlpha.300' },
            _focus: { borderColor: 'var(--accent)', ring: '1px', ringColor: 'var(--accent)' },
          },
        },
        elevated: {
          field: {
            bg: 'whiteAlpha.50',
            color: 'white',
            borderRadius: 'md',
            border: '1px solid',
            borderColor: 'whiteAlpha.100',
            transition: 'all 0.2s cubic-bezier(0.175, 0.885, 0.32, 1.275)',
            _hover: { borderColor: 'whiteAlpha.300' },
            _focus: { borderColor: 'var(--accent)', ring: '1px', ringColor: 'var(--accent)' },
          },
        },
      },
      defaultProps: {
        variant: 'elevated',
      },
    },
    Textarea: {
      baseStyle: {
        _placeholder: { color: 'text.muted', opacity: 1 },
      },
      variants: {
        clay: {
          bg: 'gray.800',
          color: 'clay.text',
          borderRadius: '8px',
          border: '1px solid rgba(255,255,255,0.08)',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: { borderColor: 'whiteAlpha.300' },
          _focus: { borderColor: 'var(--accent)', ring: '1px', ringColor: 'var(--accent)' },
        },
        elevated: {
          bg: 'whiteAlpha.50',
          color: 'white',
          borderRadius: 'md',
          border: '1px solid',
          borderColor: 'whiteAlpha.100',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: { borderColor: 'whiteAlpha.300' },
          _focus: { borderColor: 'var(--accent)', ring: '1px', ringColor: 'var(--accent)' },
        },
      },
      defaultProps: {
        variant: 'elevated',
      },
    },
    FormLabel: {
      baseStyle: {
        fontFamily: 'heading',
        fontSize: 'sm',
        color: 'gray.200',
        mb: 2,
        fontWeight: 'medium',
      },
    },
    Button: {
      variants: {
        primary: {
          bg: 'var(--accent)',
          color: 'var(--accent-foreground, #000000)',
          borderRadius: 'md',
          border: '1px solid',
          borderColor: 'whiteAlpha.300',
          boxShadow: 'panel-sm',
          transition: 'border-color 0.15s ease, box-shadow 0.15s ease',
          _hover: {
            bg: 'var(--accent)',
            borderColor: 'whiteAlpha.500',
            boxShadow: 'panel-hover',
            _disabled: { borderColor: 'whiteAlpha.300', boxShadow: 'panel-sm' },
          },
          _active: { bg: 'var(--accent)', boxShadow: 'none' },
          _focusVisible: {
            ring: '2px',
            ringColor: 'whiteAlpha.900',
            ringOffset: '2px',
            ringOffsetColor: 'var(--bg-main)',
          },
        },
        clay: {
          bg: 'clay.out',
          color: 'clay.text',
          boxShadow: 'clay-out',
          borderRadius: '8px',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: {
            bg: 'color-mix(in srgb, var(--bg-element), white 15%)',
            boxShadow: 'clay-out-hover',
            _disabled: { bg: 'clay.out' },
          },
          _active: {
            bg: 'clay.in',
            boxShadow: 'clay-in',
            transform: 'translateY(1px)',
          },
          _focusVisible: {
            ring: '2px',
            ringColor: 'var(--accent)',
            ringOffset: '1px',
            ringOffsetColor: 'gray.900',
          },
        },
        'clay-ghost': {
          bg: 'transparent',
          color: 'clay.dim',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: {
            bg: 'whiteAlpha.50',
            color: 'clay.text',
          },
          _active: {
            bg: 'whiteAlpha.100',
            transform: 'translateY(1px)',
          },
          _focusVisible: {
            ring: '2px',
            ringColor: 'var(--accent)',
            ringOffset: '1px',
            ringOffsetColor: 'gray.900',
          },
        },
        elevated: {
          bg: 'whiteAlpha.100',
          color: 'whiteAlpha.900',
          borderRadius: 'md',
          boxShadow: 'panel-sm',
          border: '1px solid',
          borderColor: 'whiteAlpha.100',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: {
            bg: 'whiteAlpha.200',
            transform: 'translateY(-1px)',
            boxShadow: 'panel-hover',
            _disabled: {
              bg: 'whiteAlpha.100',
              transform: 'none',
              boxShadow: 'panel-sm',
            }
          },
          _active: { transform: 'translateY(0)' },
          _focusVisible: {
            ring: '2px',
            ringColor: 'var(--accent)',
            ringOffset: '1px',
            ringOffsetColor: 'gray.900',
          },
        },
        destructive: {
          bg: 'red.900',
          color: 'red.100',
          transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
          _hover: { bg: 'red.800', transform: 'translateY(-1px)' },
          _active: { transform: 'translateY(0)' },
          _focusVisible: {
            ring: '2px',
            ringColor: 'red.400',
            ringOffset: '1px',
            ringOffsetColor: 'gray.900',
          },
        },
      },
      defaultProps: {
        variant: 'elevated',
      },
    },
  },
})
