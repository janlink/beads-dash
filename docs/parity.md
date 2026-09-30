# bdui-next parity checklist

Walk of the bdui-next 0.5.0 inventory ("Parity:" bullets plus dialogs, keys, theme, export, notifications, config) against bdash. Status values:

- done: implemented and covered by a test or golden.
- dropped: deliberately not carried over, per the bdash spec.
- human-only: implemented, but the last check needs a real terminal, OS or remote service.

Statuses come from the code, key map, command table and test inventory of the repository. The weaknesses the inventory lists ("W:") are fixed by design unless a row says otherwise.

## Views

| Item | Status | Note |
| --- | --- | --- |
| Tree: roots are issues whose parent is not visible, pruned parents lift children | done | Tree view goldens |
| Tree: row grid (gutter, status glyph or caret, ID with stems, type, title, meta) | done | |
| Tree: epics bold, closed rows dim, header row | done | |
| Tree keys: j/k, expand and collapse, z fold all, Enter opens, e edit, x export | done | `z M` and `z R` fold all and unfold all; `x` is the export dialog |
| Tree: detail beside the list on wide terminals, replaces it on narrow ones | done | breakpoints differ from bdui-next, see Responsive |
| Tree: header stats, direct-children progress | done | progress shows direct children and descendants |
| Tree is the default view | dropped | Overview is view 1 and the default, Tree is view 2 |
| List view | dropped | removed in bdui-next 0.3.0 too |
| Kanban: Open, In Progress, Blocked, Closed, Other columns | done | |
| Kanban: three-row cards, selected band, column rule with count | done | |
| Kanban: empty and collapsed column as spine | done | |
| Kanban keys: h/l column, j/k card, first and last | done | first and last work in every view |
| Stats view | done | replaced by Overview, which states its scope in the header |
| Memories: lazy load, select, forget with confirm, reload | done | plus create, edit, copy and search |
| Detail panel: title, metadata grid, progress, Markdown description, m toggles source, paging | done | |
| Detail panel: subtasks capped with "n more", local timestamps | done | |
| Detail panel: notes, design, acceptance, comments, history, linked issues with jump and back | done | fixes the W list of the inventory |
| Graph view | done | returned with a layout that honours filter and visibility; bdui-next removed it |

## Chrome and layout

| Item | Status | Note |
| --- | --- | --- |
| Header rule with view, workspace, stats, live or stale marker | done | stale shows the error and its age |
| Fork junctions where the side panel starts | done | |
| Footer tabs, filter note, hints that drop in a fixed order | done | hints are generated from the key map |
| Toast row with ASCII icons | done | notices queue instead of replacing each other |
| Docked search, filter and command bars | done | |
| Kanban column count by width, panel beside at wide terminals | done | breakpoints follow bdash ADR and goldens |
| Minimum size guard | done | "too small" screen below the minimum size |
| Window title push and pop | dropped | per R12: bdash leaves the terminal title alone |

## Dialogs and forms

| Item | Status | Note |
| --- | --- | --- |
| One dialog frame: title, aside, hints | done | |
| Create form | done | overlay, no confirm, parent and dependency pickers, multi-line description |
| Edit form with changed-field marker and change count | done | conflict detection on submit |
| Confirm dialog, default no for destructive actions | done | |
| Export dialog | done | sets, formats, comments, clipboard or file, preview |
| Undo (u, Ctrl+Z) | dropped | a stub in bdui-next; nothing is reverted |
| Theme selector | done | live preview, Esc reverts, persisted |
| Status visibility panel | done | merged with filter and scope |
| Filter panel | done | |
| Search with facets | done | |
| Help generated from the key map | done | |
| Command bar | done | commands resolve first, `:go <id>` jumps, completion and history |

## Keys

| Item | Status | Note |
| --- | --- | --- |
| Global keys: q, ?, Esc, r, /, f, c, :, N, Enter, t, n, m, 1-6 | done | |
| `g` as command bar | dropped | `:` opens the bar, `gg` goes to the top |
| Per-view e and x scopes | done | one current issue shared by all views |
| Declarative key map with conflict test | done | |
| Key remapping via config | dropped | not in v1; the map is declarative and tested |

## Theme and terminal

| Item | Status | Note |
| --- | --- | --- |
| Role tokens, five palettes, colour depth, NO_COLOR | done | truecolor added |
| Glyph tiers fancy, safe, ascii | done | `BDASH_GLYPHS`, `:glyphs` |
| Ambiguous-width handling | human-only | needs terminals with wide ambiguous characters |
| Light backgrounds | done | |

## Export

| Item | Status | Note |
| --- | --- | --- |
| Markdown, JSON and text formats | done | Markdown and text show titles and statuses of related issues |
| JSON is bd's own object | done | comments are added under a `comments` key as bd returns them |
| Clipboard | done | OSC 52 plus native routes by argv, result reported |
| Clipboard on WSL | done | wl-copy, xclip, xsel, pbcopy, OSC 52 and clip.exe (UTF-16LE) as the WSL route; PowerShell is never started |
| File export without clobbering | done | export.dir, atomic write, overwrite prompt, keep-both numbering |
| Export of the filtered set | done | current, marked or scope, in the source view's order |
| Export on a real clipboard | human-only | needs a desktop session or terminal with OSC 52 |

## Notifications

| Item | Status | Note |
| --- | --- | --- |
| Notify on closed | done | |
| Notify on blocked by dependency | done | fires on presentation-status changes |
| Batching, own edits suppressed | done | |
| Toggle, persisted | done | `N`, `:notify` |
| Native toast on Windows via PowerShell | dropped | scope change: no PowerShell is started |
| Native notifications on a real desktop | human-only | |

## Config, CLI and bd boundary

| Item | Status | Note |
| --- | --- | --- |
| TOML config, flag > env > config > default | done | |
| Flags --view, --no-mouse, --version (with bd support check), path argument | done | |
| Poll interval env | dropped | refresh is event-driven with a safety poll, tuned in the config |
| bd called by argv, timeouts, output caps | done | |
| Last good snapshot kept on error, stale state | done | |
| Change badges after refresh | done | |
| Mutations run in the discovered workspace | done | |
| Custom statuses and types from `bd statuses` and `bd types` | done | |
| All edge types, due and defer dates | done | |
| bd 1.3.0 events | done | opt-in journal |
| PTY end-to-end run | human-only | scripted demo and live terminal smoke |
