# bdash

A real-time terminal dashboard for Beads (`bd`) workspaces. It reads and writes issues only through the public `bd --json` CLI.

This file is a glossary. It records what the project's words mean, not how anything is built.

## Language

### Workspace state

**Snapshot**:
The whole workspace as bdash last read it in one refresh: every issue, its edges, and which issues bd calls ready or blocked. Views draw only from the current snapshot.
_Avoid_: state, cache

**Stale**:
The latest refresh failed, so bdash still shows the previous snapshot and says how old it is.
_Avoid_: offline, outdated

**Ready**:
An issue bd lists as ready to work on. bdash takes bd's answer as given and never works it out itself.
_Avoid_: unblocked, available

**Blocked**:
An issue bd lists as blocked by an unresolved dependency (including one inherited from its parent), or one whose status is `blocked`.
_Avoid_: waiting, stuck

**Presentation status**:
The status bdash shows: Open, In progress, Blocked, Frozen, Closed or Other. It comes from the category bd gives the issue's status plus whether bd calls the issue blocked. An in-progress issue stays In progress when blocked and carries a blocked marker.
_Avoid_: column status, display status

**Frozen**:
The presentation status of issues deliberately parked, such as deferred or pinned ones.
_Avoid_: on hold, paused

**Closed**:
The presentation status of issues whose status bd counts as done.
_Avoid_: done, finished (as presentation names)

**Progress**:
How far an issue with children has come: closed direct children out of all direct children, plus a bar over all descendants. Frozen and Other children still count as not closed. Issues without children show no progress.
_Avoid_: completion, percent done

**Container**:
An issue with children, such as an epic. Containers track work rather than being worked on, so lists of ready work leave them out.
_Avoid_: parent issue (when the point is that it holds work), umbrella

**Activity event**:
One change bdash noticed between two snapshots, such as created, claimed, closed, became blocked or priority changed, with the issue, the time and, when known, who did it. Dependency edges produce no events of their own; their effect shows as became blocked or unblocked.
_Avoid_: change, update, event (bare)

**Activity feed**:
The newest activity events, newest first, including events rebuilt at startup from what the first snapshot already tells.
_Avoid_: log, history, timeline

**Change highlight**:
The short-lived marker on an issue that had an activity event in a recent refresh. It counts down only while the viewer can see bdash, and the issue's details say which events caused it.
_Avoid_: badge, flash, recent change

**Needs attention**:
The short list on the Overview of issues someone should look at now: the top ready issues that are not containers, the top blocked ones, and how many are assigned but not started.
_Avoid_: todo, inbox

### Memories

**Memory**:
A keyed note stored in the workspace that bd hands to every agent session. It has a key and content, but no author, time or order.
_Avoid_: note, kv entry

**Memory key**:
The name a memory is stored under. It is case-sensitive and free-form; renaming a memory means storing it under the new key and forgetting the old one.
_Avoid_: slug

**Current memory**:
The one memory the Memories view points at, separate from the current issue.
_Avoid_: selected memory

### People

**Active assignee**:
Anyone whose name is the assignee of at least one in-progress issue. Human or agent alike: Beads records only a name, so bdash does not tell them apart.
_Avoid_: agent, active agent, worker

### Interaction

**Current issue**:
The one issue every view points at. It follows the viewer across views when the issue is visible there, and every issue action (details, quick change, export, command bar) acts on it.
_Avoid_: selection, selected issue, cursor

**Marked issues**:
Issues the viewer has set aside for one action on all of them at once, such as a quick change or an export. Marking does not move the current issue.
_Avoid_: multi-selection, checked issues

**Detail panel**:
Everything about the current issue in one place, beside or below the view or over it. Beside or below, it follows the current issue while the view keeps the keys; over the view, it takes them.
_Avoid_: inspector, preview, sidebar

**Audit trail**:
What happened to one issue over time: its comments and the changes recorded to it. The recorded changes carry no author.
_Avoid_: log, history (bare), timeline

**Dialog**:
A focused interaction drawn over the dimmed view, such as the issue form, a picker or a confirmation. Dialogs stack: the one on top takes the keys.
_Avoid_: modal, popup, overlay (as nouns for the concept)

**Docked bar**:
An input strip under the footer (search, command bar, filter) that shrinks the view instead of covering it, so the view stays live while the viewer types.
_Avoid_: panel, dock

**Issue form**:
The one dialog that both creates and edits an issue. In edit mode it marks changed fields and writes only those.
_Avoid_: create dialog, edit dialog (as separate things)

**Issue picker**:
A dialog for choosing one or more issues by fuzzy-matching id and title, used for parent, dependencies and jumping.

**Quick change**:
Changing a single field of the current issue (status, priority, assignee, labels, or closing it) from a small menu, without opening the issue form.

**Edit conflict**:
The issue being edited changed in the workspace after the issue form opened. bdash shows what changed and lets the viewer keep or drop their own edits.

**Destructive action**:
An action that cannot be undone by another bd write, such as forgetting a memory or discarding unsaved form edits. Only destructive actions ask for confirmation; closing an issue is not one, because it can be reopened.

### Scope

**Scope**:
Which issues the views currently show: status visibility, facets and free text taken together. There is one scope for the whole session, shared by every view except Memories; search and filter are the two ways to edit it, not separate states.
_Avoid_: filter, search (as names for the state)

**Facet**:
A structured term in the scope that matches one issue attribute, such as a label, type, priority, status, assignee or parent. Terms in one category widen the scope; different categories narrow it.
_Avoid_: tag, criterion

**Status visibility**:
Which presentation statuses the scope lets through. Closed is hidden unless the viewer asks for it.
_Avoid_: status filter

### Export

**Export**:
A rendering of a set of issues in one format (Markdown, JSON or plain text), sent to the clipboard or a file.
_Avoid_: dump, report, backup (bd's word)

**Export set**:
The issues an export covers: the current issue, the marked issues, or everything the scope lets through.
_Avoid_: selection

### Dependencies

**Waits on**:
The issues that block an issue, directly or through a chain of blockers. Parent links do not count.
_Avoid_: upstream, depends on (as a display label)

**Holds up**:
The issues an issue blocks, directly or through a chain. The reverse of waits on.
_Avoid_: downstream, blocks (as a display label)

**Entry child**:
A child whose blockers all lie outside its parent's other children, so it can start as soon as the parent's own blockers allow. The dependency graph draws a parent's link only to its entry children, because the other children are already reached through their blockers.

**Dependency graph**:
The view that shows how the workspace's issues wait on each other: one outline per group of connected issues, each issue once, later sightings as references back to it. Issues with no blockers and nothing they hold up are left out and only counted.
_Avoid_: DAG, graph view (bare), network

**Focus graph**:
What one issue waits on and what it holds up, shown in its detail panel as two outlines around it, limited to a few steps deep.
_Avoid_: local graph, mini graph

### Configuration

**Setting**:
A preference bdash remembers between runs, such as the theme or the default view. Choosing one in bdash saves it; the viewer may also edit it by hand.
_Avoid_: option, preference (as nouns for the concept)

**Override**:
A flag or environment variable that beats a setting for one run without changing it. While an override is active, bdash says so where the setting is chosen.
_Avoid_: forced setting

### Appearance

**Theme**:
A named set of colours for every role bdash draws (status, priority, type, text, chrome), such as Default, Ocean or Monochrome. Each theme has a light and a dark variant.
_Avoid_: palette, colour scheme

**Background mode**:
Which variant of the theme applies: dark, light, or auto (follow the terminal's background).
_Avoid_: appearance, light/dark theme

**Colour depth**:
How many colours bdash may use: truecolor, 256, 16, or none. With none, hierarchy is carried by bold, faint and inverse text only.

**Glyph tier**:
The set of characters bdash draws with: fancy, safe (a subset old terminal fonts carry), or ascii. It is the viewer's choice, because font coverage cannot be detected.
_Avoid_: icon set, charset

**Appearance dialog**:
The dialog for choosing the theme and glyph tier, previewed live on the view behind it.
_Avoid_: theme selector
