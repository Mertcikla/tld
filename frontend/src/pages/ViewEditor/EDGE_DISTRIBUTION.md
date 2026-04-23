# Edge Distribution System (ViewEditor)

This system prevents connectors from overlapping when sharing nodes or handles. It distributes overlapping edges spatially, aligns them to real React Flow handles, and sorts them to minimize crossings.

## Source Files

- **Data Orchestration**: `src/pages/ViewEditor/hooks/useViewData.ts`
- **Curved Edges**: `src/components/ViewBezierConnector.tsx`
- **Straight Edges**: `src/components/ContextStraightConnector.tsx`
- **Ghost Edges**: `src/components/ProxyConnectorEdge.tsx`

---

## 1. Data Layer Logic (`useViewData.ts`)

The grouping logic is located in the `useEffect` that derives `rfEdges`.

### Grouping Mechanism
Instead of grouping by edge corridors (source-target pairs), we group by **individual handle usage**.
1. We collect all connectors attached to each specific `elementId-handle` (e.g., `123-right`).
2. Both `source` and `target` usages are tracked in the same pool for that handle.
3. Each usage record stores the `otherNodeCoord` (the Y or X position of the node at the other end).

### Spatial Sorting
To minimize crossings near the handle, members sharing a handle are sorted based on their `otherNodeCoord`:
- **Left / Right handles**: Sorted by the `Y` coordinate of the connected node.
- **Top / Bottom handles**: Sorted by the `X` coordinate of the connected node.

### Edge Data Payload
Each edge is enriched with distribution metadata in its `data` object:
- `sourceGroupIndex` / `sourceGroupCount`
- `targetGroupIndex` / `targetGroupCount`
- `sourceHandleSide` / `targetHandleSide`
- `sourceHandleSlot` / `targetHandleSlot`

### Visual Handle Mapping
- The backend still stores logical handles as `top`, `right`, `bottom`, `left`.
- React Flow renders each logical side as a capped bead of **5 physical handles**: `side-0` through `side-4`.
- Group members are mapped onto those 5 slots using their sorted rank. Once a side has more than 5 edges, extra edges reuse the nearest slot instead of creating more unique handle positions.

---

## 2. Rendering Logic

### ViewBezierConnector (Curved Edges)
- Curved connectors now use the actual React Flow handle positions directly.
- `useViewData.ts` assigns each edge to a physical `side-slot` handle id so the curve endpoint already lands on the correct circle.
- Slot spacing is currently `12px`.

### Straight Connectors (Diagonal/Corridors)
Uses a perpendicular vector shift to ensure distribution works at any angle.
1. Calculate direction vector `(dx, dy)`.
2. Compute normal vector `nx = -dy / len`, `ny = dx / len`.
3. Shift the endpoint by `(offset * nx, offset * ny)`.

---

## 3. Maintenance Notes

- **Ghost Connectors**: `useViewContextNeighbours.ts` currently deduplicates ghost edges by node pairs. However, the `ProxyConnectorEdge` component supports the distribution props if deduplication is ever removed or if they overlap with standard edges.
- **Slot Count**: Adjust `HANDLE_SLOT_COUNT` in `src/utils/edgeDistribution.ts` to change the number of unique handle positions per side.
- **Slot Gap**: Adjust `HANDLE_SLOT_GAP` in `src/utils/edgeDistribution.ts` to change the spacing between circles.
- **Labeling**: Labels are calculated based on the shifted coordinates/path to ensure they stay centered on the distributed line.
- **Selection**: When an edge is selected, the physical handles used by that edge grow in size so reconnection targets stay obvious.
