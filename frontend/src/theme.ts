import { extendTheme, type ThemeConfig } from '@chakra-ui/react'

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
      border: 'rgba(255, 255, 255, 0.08)',
    },
  },
  shadows: {
    // Semantic shadows for glass panels
    'panel': '0 8px 32px rgba(0,0,0,0.5)',
    'panel-sm': '0 4px 10px rgba(0,0,0,0.3)',
    'panel-hover': '0 6px 15px rgba(0,0,0,0.4)',
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
        backdropFilter: 'blur(16px)',
        border: '1px solid',
        borderColor: 'glass.border',
        boxShadow: 'panel',
      },
    },
  },
  components: {
    Modal: {
      baseStyle: {
        dialog: {
          bg: 'clay.bg',
          borderRadius: '16px',
          boxShadow: 'clay-out',
          border: '1px solid rgba(255,255,255,0.06)',
        },
        header: {
          color: 'gray.100',
          fontSize: 'lg',
          fontWeight: 'bold',
          borderBottom: '1px solid rgba(255,255,255,0.06)',
        },
        body: {
          color: 'gray.300',
        },
        footer: {
          borderTop: '1px solid rgba(255,255,255,0.06)',
        },
      },
    },
    Drawer: {
      baseStyle: {
        dialog: {
          bg: 'clay.bg',
          borderLeft: '1px solid rgba(255,255,255,0.06)',
          boxShadow: 'clay-out',
        },
      },
    },
    Menu: {
      baseStyle: {
        list: {
          bg: 'clay.bg',
          boxShadow: 'clay-out',
          border: '1px solid rgba(255,255,255,0.08)',
          borderRadius: '12px',
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
        bg: 'clay.out',
        color: 'clay.text',
        fontSize: '12px',
        borderRadius: '8px',
        px: 3,
        py: 1.5,
        boxShadow: '0 8px 30px rgba(0,0,0,0.6)',
        border: '1px solid rgba(255,255,255,0.06)',
        zIndex: 99999,
        arrow: {
          bg: 'clay.out',
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
