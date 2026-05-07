# Density Slider with Per-View Visibility Overrides

## Summary
Implement a persisted `-2..2` density system for every view, plus element+connector promote/demote controls in ViewEditor. Density remains a read-time projection: moving the slider or changing overrides does not delete or rematerialize generated objects. Default density is `0`.

Density levels:
- `-2 Essential`: soft target 4 elements / 8 connectors.
- `-1 Compact`: soft target 8 / 16.
- `0 Balanced`: soft target 12 / 24.
- `1 Expanded`: soft target 32 / 64.
- `2 Full`: no projection cap.

## Key Changes
- Add persisted view density:
  - Add `views.density_level INTEGER NOT NULL DEFAULT 0`.
  - Add local JSON endpoints:
    - `GET /api/views/{id}/density`
    - `PUT /api/views/{id}/density { density_level }`
  - Validate density is `-2..2`.

- Add per-view object overrides:
  - Add `view_visibility_overrides` table keyed by `view_id`, `resource_type` (`element` or `connector`), and `resource_id`.
  - Store `level_delta INTEGER NOT NULL DEFAULT 0`, clamped to `-4..4`, plus timestamps.
  - Positive delta promotes visibility; negative delta demotes visibility.
  - Effective level is computed from backend semantic/structural score plus delta.
  - Delta `0` removes the override row.

- Add override APIs:
  - `GET /api/views/{id}/visibility-overrides`
  - `PUT /api/views/{id}/visibility-overrides { resource_type, resource_id, level_delta }`
  - `POST /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}/promote`
  - `POST /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}/demote`
  - `DELETE /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}`

- Add density projection:
  - Project placements/connectors at read time using density level, soft caps, and overrides.
  - Watch-backed views rank with `watch_filter_decisions`, fact confidence, materialization owner type, and architecture link confidence.
  - Manual/non-watch views rank structurally using degree, connectivity, selected/focused state, and connector endpoint preservation.
  - Connectors are visible only when both endpoints are visible, unless projection promotes a missing endpoint to preserve a user-promoted connector.

- Update ViewEditor:
  - Show a Density slider/segmented control for every view.
  - Add quick Promote / Demote / Reset controls in `ElementPanel` and `ConnectorPanel`.
  - Promoted/demoted objects get a small visual indicator in the panel and optionally on-canvas.
  - Replace view-level Show Context / Clean Noise with Density for the main toolbar.
  - Keep element-level Hide Context only as an advanced watch-specific action.

## Behavior Rules
- Overrides are scoped to the current view only.
- Promote/demote is relative, not an exact level picker.
- Level `2 Full` shows all current view content, except explicit demotions may still hide objects unless the user resets overrides.
- Promoted connectors pull in endpoints if needed.
- Manual edits and generated watch rematerialization must not erase override rows unless the view or resource is deleted.
- Existing enricher metadata stays unchanged.

## Test Plan
- Backend tests:
  - Density and override validation.
  - Promote/demote clamps deltas and reset removes rows.
  - Projection applies overrides after base scoring.
  - Promoted connector preserves both endpoints.
  - Demoted element hides incident connectors unless those connectors are separately promoted.
  - Manual views use structural projection without watch metadata.
  - Watch views use filter decisions and confidence inputs.

- Integration tests:
  - Persisted density survives reload.
  - Overrides survive watch re-representation.
  - Deleting a view/resource cleans override rows.
  - `2 Full` returns full content except explicit demotions.

- Frontend tests:
  - Density slider loads/saves per view.
  - Element and connector panels promote/demote/reset objects.
  - View content refreshes after density or override changes.
  - Override indicators render for promoted/demoted objects.

## Assumptions
- Use local JSON endpoints to avoid proto churn.
- Per-view persistence is required.
- Overrides are current-view scoped.
- Promote/demote uses relative deltas.
- Soft caps are preferred over hard caps.
- `watch_filter_decisions` becomes projection input, but remains the explainability/audit trail.
