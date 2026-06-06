import React, { useEffect, useRef } from "react"
import type { TopMenuBarSlots } from '../slots'
import { Link as RouterLink, useLocation, useNavigate } from "react-router-dom"
import {
  Box,
  Flex,
  HStack,
  IconButton,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
  Portal,
  Text,
  Tooltip,
  useDisclosure,
  useMediaQuery,
} from "@chakra-ui/react"
import { SettingsIcon } from "@chakra-ui/icons"
import logoMarkUrl from "../assets/logo-mark.svg"
import { useAccentColor } from "../context/ThemeContext"
import { hexToRgba } from "../constants/colors"
import AppearanceSettings from "../pages/AppearanceSettings"
import ExperimentalSettings from "../pages/ExperimentalSettings"
import UpdateSettings from "../pages/UpdateSettings"
import { isWailsApp, isWailsMac, isWailsWindows } from "../config/runtime"
import WindowsWindowControls from "./WindowsWindowControls"

const FolderTreeIcon = ({ size = 32 }: { size?: number }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    <rect x="8" y="2" width="8" height="5" rx="1.5" fill="none" />
    <line x1="12" y1="7" x2="12" y2="11.5" />
    <line x1="4.5" y1="11.5" x2="19.5" y2="11.5" />
    <line x1="4.5" y1="11.5" x2="4.5" y2="14" />
    <rect x="1.5" y="14" width="6" height="4.5" rx="1.5" fill="none" />
    <line x1="19.5" y1="11.5" x2="19.5" y2="14" />
    <rect x="16.5" y="14" width="6" height="4.5" rx="1.5" fill="none" />
  </svg>
)

const PencilIcon = ({ size = 24 }: { size?: number }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeWidth="2">
    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
  </svg>
)


const InventoryIcon = ({ size = 22 }: { size?: number }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
    <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
    <line x1="12" y1="22.08" x2="12" y2="12" />
  </svg>
)

interface Props extends TopMenuBarSlots {
  children?: React.ReactNode
  hideMobileBar?: boolean
}

type NavItem = {
  label: string
  path: string
  icon: (props: { size?: number }) => JSX.Element
  shortcutKey: "1" | "2" | "3"
}

const NAV_ITEMS: NavItem[] = [
  { label: "Editor", path: "/", icon: PencilIcon, shortcutKey: "1" },
  { label: "Diagrams", path: "/views", icon: FolderTreeIcon, shortcutKey: "2" },
  { label: "Inventory", path: "/inventory", icon: InventoryIcon, shortcutKey: "3" },
]

const NAV_SHORTCUT_PATHS: ReadonlyMap<string, string> = new Map(
  NAV_ITEMS.map((item) => [item.shortcutKey, item.path]),
)

export default function TopMenuBar({
  children,
  hideMobileBar,
  rightSlot,
  mobileMenuSlot,
  settingsSlot,
  mobileSettingsSlot,
  userControlsSlot,
}: Props) {
  const location = useLocation()
  const navigate = useNavigate()
  const { accent } = useAccentColor()
  const [isSmallerThan1150] = useMediaQuery("(max-width: 1150px)")
  const [isMobileLayout] = useMediaQuery("(max-width: 479px)")
  const appearancePopover = useDisclosure()
  const appearanceTriggerRef = useRef<HTMLButtonElement | null>(null)
  const appearanceContentRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!appearancePopover.isOpen) return

    const closeOnOutsideInteraction = (event: MouseEvent | TouchEvent | PointerEvent) => {
      const target = event.target
      if (!(target instanceof Node)) return
      if (appearanceTriggerRef.current?.contains(target)) return
      if (appearanceContentRef.current?.contains(target)) return
      appearancePopover.onClose()
    }

    document.addEventListener("pointerdown", closeOnOutsideInteraction, true)
    document.addEventListener("mousedown", closeOnOutsideInteraction, true)
    document.addEventListener("touchstart", closeOnOutsideInteraction, true)
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideInteraction, true)
      document.removeEventListener("mousedown", closeOnOutsideInteraction, true)
      document.removeEventListener("touchstart", closeOnOutsideInteraction, true)
    }
  }, [appearancePopover])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const isModifierPressed = event.metaKey || event.ctrlKey
      if (!isModifierPressed || event.altKey || event.shiftKey) return

      const shortcutPath = NAV_SHORTCUT_PATHS.get(event.key)
      if (!shortcutPath) return

      event.preventDefault()
      navigate(shortcutPath)
    }

    window.addEventListener("keydown", handleKeyDown)
    return () => {
      window.removeEventListener("keydown", handleKeyDown)
    }
  }, [navigate])

  const isActive = (path: string) => {
    if (path === "/") {
      return location.pathname === "/" || location.pathname.startsWith("/views/")
    }
    if (path === "/views") {
      return location.pathname === "/views"
    }
    return location.pathname === path || location.pathname.startsWith(`${path}/`)
  }

  const renderAppearancePopover = (trigger: React.ReactElement, placement: "bottom-end" | "top-end" = "bottom-end") => (
    <Popover
      placement={placement}
      isLazy
      closeOnBlur
      isOpen={appearancePopover.isOpen}
      onOpen={appearancePopover.onOpen}
      onClose={appearancePopover.onClose}
      returnFocusOnClose={false}
    >
      <PopoverTrigger>
        {trigger}
      </PopoverTrigger>
      <Portal>
        <PopoverContent
          ref={appearanceContentRef}
          mr={{ base: 2, sm: 0 }}
          mt={2}
          w={{ base: "calc(100vw - 24px)", sm: "360px" }}
          maxW="360px"
          maxH="calc(var(--app-viewport-height) - 96px)"
          display="flex"
          flexDirection="column"
          bg="rgba(var(--bg-main-rgb), 0.95)"
          backdropFilter="blur(18px)"
          borderColor="whiteAlpha.200"
          boxShadow="0 18px 48px rgba(0,0,0,0.45)"
          borderRadius="20px"
          overflow="hidden"
          position="relative"
          _focus={{
            outline: "none",
            boxShadow: "0 18px 48px rgba(0,0,0,0.45)",
          }}
          _focusVisible={{
            outline: "none",
            boxShadow: "0 18px 48px rgba(0,0,0,0.45)",
          }}
        >
          <PopoverArrow bg="rgba(var(--bg-main-rgb), 0.95)" />
          <PopoverBody p={4} pb={7} overflowY="auto" flex={1} minH={0}>
            <Flex direction="column" gap={5}>
              <AppearanceSettings compact />
              <ExperimentalSettings compact />
              {isWailsApp && <UpdateSettings compact />}
            </Flex>
          </PopoverBody>
          <Box
            position="absolute"
            bottom={2}
            right={4}
            pointerEvents="none"
            userSelect="none"
          >
          </Box>
        </PopoverContent>
      </Portal>
    </Popover>
  )

  return (
    <>
      <Flex
        py={0}
        className="glass"
        align="center"
        direction="row"
        flexShrink={0}
        position="fixed"
        top={0}
        left={0}
        right={0}
        zIndex={1100}
        minH={{ base: "var(--topbar-h-mobile-total)", sm: "var(--topbar-h-total)" }}
        style={{
          paddingTop: "env(safe-area-inset-top, 0px)",
          paddingLeft: "max(env(safe-area-inset-left, 0px), 8px)",
          paddingRight: isWailsWindows
            ? "calc(var(--wails-window-controls-w, 138px) + max(env(safe-area-inset-right, 0px), 8px))"
            : "max(env(safe-area-inset-right, 0px), 8px)",
          "--wails-draggable": isWailsApp ? "drag" : "no-drag",
        } as React.CSSProperties}
        sx={{
          containerType: "inline-size",
          containerName: "topbar",
          "&.glass": {
            borderTop: "none",
            borderLeft: "none",
            borderRight: "none",
          },
        }}
      >
        {isWailsWindows && <WindowsWindowControls />}

        <HStack
          spacing={0}
          h="full"
          flexShrink={0}
          display={{ base: "none", sm: "flex" }}
          style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
        >
          {isWailsMac ? (
            <Box w="82px" h="full" />
          ) : (
            <HStack
              as={RouterLink}
              to="/"
              spacing={2}
              mr={4}
              cursor="pointer"
              px={2}
              py={1}
              borderRadius="xl"
              transition="all 0.2s"
              _hover={{
                bg: "whiteAlpha.100",
                "& .logo-mark": {
                  transform: "translateY(-4px) rotateX(10deg) rotateY(-10deg)",
                  filter: "drop-shadow(0 8px 8px rgba(0,0,0,0.4))",
                },
              }}
              sx={{ perspective: "1000px" }}
            >
              <Box
                as="img"
                src={logoMarkUrl}
                alt=""
                height="30px"
                display="block"
                className="logo-mark"
                transition="all 0.4s cubic-bezier(0.34, 1.56, 0.64, 1)"
              />
              <Text
                fontFamily="heading"
                fontWeight="700"
                lineHeight="1"
                mt="3px"
                sx={{
                  "@container topbar (max-width: 920px)": {
                    display: "none !important",
                  },
                }}
              >
                tlDiagram
              </Text>
            </HStack>
          )}

          <HStack spacing={2} h="full" align="center" style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
            {NAV_ITEMS.map((item) => {
              const active = isActive(item.path)
              const Icon = item.icon
              return (
                <Tooltip
                  key={item.path}
                  label={item.label}
                  placement="bottom"
                  openDelay={400}
                  isDisabled={!isSmallerThan1150}
                >
                  <Box
                    data-testid={`topnav-${item.label.toLowerCase()}`}
                    as={RouterLink}
                    to={item.path}
                    h="32px"
                    display="flex"
                    alignItems="center"
                    fontSize="13px"
                    fontWeight={active ? "700" : "500"}
                    color={active ? accent : "whiteAlpha.700"}
                    borderRadius="md"
                    bg={active ? "whiteAlpha.200" : "whiteAlpha.100"}
                    boxShadow={active ? `0 0 15px ${hexToRgba(accent, 0.4)}` : "0 4px 10px rgba(0,0,0,0.3)"}
                    border="1px solid"
                    borderColor={active ? accent : "whiteAlpha.100"}
                    _hover={{
                      bg: active ? "whiteAlpha.300" : "whiteAlpha.200",
                      transform: "translateY(-1px)",
                      boxShadow: active ? `0 0 20px ${hexToRgba(accent, 0.5)}` : "panel-hover",
                      color: "white",
                    }}
                    _active={{ transform: "translateY(0)" }}
                    transition="all 0.2s var(--chakra-transitions-easing-pop)"
                    position="relative"
                    px={4}
                    w="auto"
                    gap={2}
                    sx={{
                      "@container topbar (max-width: 1150px)": {
                        px: 2,
                        w: "36px",
                        gap: 0,
                        "& .nav-label": { display: "none" },
                      },
                    }}
                  >
                    <Icon size={18} />
                    <Box as="span" className="nav-label" lineHeight="1">{item.label}</Box>
                  </Box>
                </Tooltip>
              )
            })}
          </HStack>
        </HStack>

        {/* Centered Notch Container */}
        {children && (
          <Flex
            position="absolute"
            left="var(--topbar-notch-left, 50%)"
            transform="translateX(-50%)"
            h="full"
            align="flex-start"
            justify="center"
            pointerEvents="none"
            zIndex={5}
            display={{ base: "none", sm: "flex" }}
            sx={{
              "@container topbar (max-width: 820px)": {
                display: "none !important",
              },
            }}
          >
            <Box pointerEvents="auto" style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
              {children}
            </Box>
          </Flex>
        )}

        {/* Spacer to push right-side content */}
        <Box flex={1} minW={0} display={{ base: "none", sm: "block" }} />

        <HStack spacing={2} ml="auto" flexShrink={0} display={{ base: "none", sm: "flex" }} style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
          {rightSlot}
          {userControlsSlot}

          {settingsSlot !== undefined ? settingsSlot : (!isMobileLayout && renderAppearancePopover(
            <IconButton
              ref={appearanceTriggerRef}
              data-testid="topnav-appearance"
              aria-label="Appearance"
              icon={<SettingsIcon boxSize={4} />}
              size="sm"
              borderRadius="full"
              bg="whiteAlpha.100"
              color="whiteAlpha.700"
              border="1px solid"
              borderColor="whiteAlpha.100"
              _hover={{
                bg: "whiteAlpha.200",
                color: "white",
                transform: "translateY(-1px)",
              }}
            />,
          ))}
        </HStack>
      </Flex>

      {children && !hideMobileBar && (
        <Flex
          display={{ base: "flex", sm: "none" }}
          position="fixed"
          left="50%"
          style={{ top: "calc(env(safe-area-inset-top, 0px) + var(--topbar-float-top))" } as React.CSSProperties}
          transform="translateX(-50%)"
          zIndex={99}
          align="center"
          justify="center"
          px={2}
          py={1}
          maxW="90vw"
          minH="var(--topbar-float-h)"
          sx={{ "& > *": { filter: "drop-shadow(0 6px 18px rgba(0,0,0,0.7)) drop-shadow(0 2px 6px rgba(0,0,0,0.5))" } }}
        >
          {children}
        </Flex>
      )}

      <Box
        display={{ base: "block", sm: "none" }}
        position="fixed"
        bottom={0}
        left={0}
        right={0}
        zIndex={200}
        pointerEvents="none"
        style={{ height: "calc(var(--bottomnav-container-h) + env(safe-area-inset-bottom, 0px))" } as React.CSSProperties}
      >
        <Box
          position="absolute"
          bottom={0}
          left={0}
          right={0}
          bg="var(--bg-header)"
          backdropFilter="blur(20px)"
          borderTop="1px solid"
          borderColor="whiteAlpha.200"
          boxShadow="0 -4px 32px rgba(0,0,0,0.5)"
          pointerEvents="auto"
          style={{ height: "calc(var(--bottomnav-h) + env(safe-area-inset-bottom, 0px))" } as React.CSSProperties}
        />

        <Flex
          position="absolute"
          bottom={0}
          left={0}
          right={0}
          align="center"
          style={{
            height: "var(--bottomnav-h)",
            paddingBottom: "env(safe-area-inset-bottom, 0px)",
            paddingLeft: "max(env(safe-area-inset-left, 0px), 4px)",
            paddingRight: "max(env(safe-area-inset-right, 0px), 4px)",
          } as React.CSSProperties}
        >
          {[
            { label: "Editor", path: "/", icon: PencilIcon },
            { label: "Diagrams", path: "/views", icon: FolderTreeIcon },
            { label: "Inventory", path: "/inventory", icon: InventoryIcon },
          ].map((item) => {
            const Icon = item.icon
            const active = isActive(item.path)
            return (
              <Box
                key={item.path}
                data-testid={`mobile-topnav-${item.label.toLowerCase()}`}
                as={RouterLink}
                to={item.path}
                flex={1}
                display="flex"
                flexDir="column"
                alignItems="center"
                justifyContent="center"
                gap="3px"
                h="full"
                color={active ? "var(--accent)" : "whiteAlpha.500"}
                transition="color 0.15s"
                pointerEvents="auto"
                _active={{ opacity: 0.6 }}
                style={{ WebkitTapHighlightColor: "transparent" } as React.CSSProperties}
              >
                <Icon size={20} />
                <Text fontSize="9px" fontWeight={active ? "700" : "500"} letterSpacing="wide" textTransform="uppercase">
                  {item.label}
                </Text>
              </Box>
            )
          })}
          {mobileMenuSlot}
          {mobileSettingsSlot !== undefined ? mobileSettingsSlot : (isMobileLayout && renderAppearancePopover(
            <Box
              ref={appearanceTriggerRef}
              data-testid="mobile-topnav-appearance"
              as="button"
              type="button"
              aria-label="Appearance"
              flex={1}
              display="flex"
              flexDir="column"
              alignItems="center"
              justifyContent="center"
              gap="3px"
              h="full"
              color={appearancePopover.isOpen ? "var(--accent)" : "whiteAlpha.500"}
              transition="color 0.15s"
              pointerEvents="auto"
              _active={{ opacity: 0.6 }}
              style={{ WebkitTapHighlightColor: "transparent" } as React.CSSProperties}
            >
              <SettingsIcon boxSize={5} />
              <Text fontSize="9px" fontWeight={appearancePopover.isOpen ? "700" : "500"} letterSpacing="wide" textTransform="uppercase">
                Settings
              </Text>
            </Box>,
            "top-end",
          ))}
        </Flex>
      </Box>
    </>
  )
}
