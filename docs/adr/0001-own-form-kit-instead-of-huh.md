# Own form kit on Bubbles instead of Huh

The baseline plan named Huh for bdash's create/edit dialogs and filter panel. We build a small form kit of our own on Bubbles v2 (`textinput`, `textarea`, `viewport`) instead. Huh's last tag is months old with fixes unreleased on main, its embedding model is still v1-shaped, and it brings a second theme system beside bdash's role tokens. The dialogs also need behaviour that fights a generic form library: change markers in edit mode, a live edit-conflict banner, issue pickers fed from the snapshot, and key hints generated from bdash's own key map. The kit needs only five field kinds, so owning it costs less than bending Huh.

## Considered options

- **Huh v2:** ready-made forms, but pinning a main pseudo-version, theming twice, and Windows Tab/Enter issues (huh#286).
- **Hybrid** (Huh for simple dialogs, custom for the rest): two form models to style, test and keep consistent.
