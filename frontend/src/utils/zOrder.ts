// Canvas stacking order for the view editor.
//
// React Flow paints connectors in one <svg> layer per distinct z-index and
// elements as absolutely-positioned siblings in the same stacking context, so
// these values decide what occludes what.
//
// Default: elements sit above connectors and their labels. Hovering or
// selecting an element lifts its connectors above other elements; hovering or
// selecting a single connector lifts that connector (and its label) instead.

export const Z_CONNECTOR = 0
export const Z_CONNECTOR_LABEL = 1
export const Z_CONTEXT_BOUNDARY = 2

export const Z_ELEMENT = 100
export const Z_ELEMENT_LAYER_HIGHLIGHT = 110
export const Z_ELEMENT_VERSION_PULSE = 120
export const Z_CONTEXT_NODE = 130
export const Z_CONTEXT_GROUP_ANCHOR = 140

export const Z_CONNECTOR_ACTIVE = 200
export const Z_CONNECTOR_LABEL_ACTIVE = 210
export const Z_ELEMENT_ACTIVE = 300

export const Z_ELEMENT_INTERACTION_SOURCE = 1000
export const Z_CONNECTOR_PREVIEW = 1500
export const Z_ELEMENT_PENDING = 2000
