# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - Unreleased

First release: a Go rewrite of bdui-next.

### Added

- Views: Overview, Tree, Kanban, Ready, Memories and the dependency graph, with a detail panel beside or over them.
- Live refresh from bd with a stale state that keeps the last good snapshot.
- Search with facets, filter, status visibility, command bar and issue picker.
- Writes through bd: create, edit, status, priority, assignee, labels, dependencies, close and reopen, single and bulk, with conflict detection.
- Memories: list, create, edit, forget and copy.
- Export of the current, marked or scoped issues as Markdown, text or JSON (bd's own objects plus comments), to the clipboard or a file, from the `x` dialog or `:export`.
- Clipboard through OSC 52 and native tools, and notifications for closed, blocked and ready issues.
- Themes, glyph tiers, colour depths and a TOML config file.
- Release artefacts: archives for linux and darwin, a Windows executable, checksums, SBOM, build provenance and a Homebrew formula.
