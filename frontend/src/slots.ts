/**
 * @tldiagram/core-ui Extension Point Slots
 *
 * Slots are React.ReactNode props that host applications inject into core
 * components to layer in optional host-app features (sharing, extra tools, etc.)
 * without modifying the core library.
 *
 * Naming convention: `<area>Slot` describes WHERE the content renders.
 */
import type { ReactNode } from 'react'

// ─── TopMenuBar ──────────────────────────────────────────────────────────────

export interface TopMenuBarSlots {
  /**
   * Rendered in the right section of the desktop top bar, before the
   * settings / user controls. Use for share buttons,
   * buttons, notification bells, etc.
   *
   * @example <AvatarGroup /> or <ShareButton />
   */
  rightSlot?: ReactNode

  /**
   * Rendered inside the mobile "Options" bottom-nav menu, after the
   * default settings items. Use for custom menu items, links, etc.
   *
   * @example <MenuItem>My Custom Action</MenuItem>
   */
  mobileMenuSlot?: ReactNode

  /**
   * Overrides the default user profile / settings controls.
   * If provided, the default user avatar will be hidden.
   */
  userControlsSlot?: ReactNode
}

// ─── ElementPanel ────────────────────────────────────────────────────────────

export interface ElementPanelSlots {
  /**
   * Rendered at the bottom of the ElementPanel body.
   */
  elementPanelAfterContentSlot?: ReactNode
}

// ─── ConnectorPanel ──────────────────────────────────────────────────────────

export interface ConnectorPanelSlots {
  /**
   * Rendered at the bottom of the ConnectorPanel body.
   */
  connectorPanelAfterContentSlot?: ReactNode
}

// ─── ViewFloatingMenu ────────────────────────────────────────────────────────

export interface ViewFloatingMenuSlots {
  /**
   * Overrides or wraps the default Share button.
   */
  shareSlot?: ReactNode

  /**
   * Rendered inside the ViewFloatingMenu, after the existing toolbar
   * buttons. Use for extra buttons, export to SaaS, etc.
   */
  toolbarSlot?: ReactNode
}

// ─── ViewEditor ──────────────────────────────────────────────────────────────

export interface ViewEditorSlots {
  /**
   * Rendered as a fixed overlay on top of the ReactFlow canvas.
   */
  canvasOverlaySlot?: ReactNode
}

// ─── Combined ────────────────────────────────────────────────────────────────

/**
 * All extension points collected. Host apps can build a single
 * "slots" object and pass it down via context or props.
 */
export interface CoreUISlots
  extends TopMenuBarSlots,
  ElementPanelSlots,
  ConnectorPanelSlots,
  ViewFloatingMenuSlots,
  ViewEditorSlots { }
