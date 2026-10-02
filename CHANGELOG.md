# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-10-02

First public release.

### Added

- Menu bar item with CPU, memory, network, temperature, battery and free disk space; compact text,
  small graphs, one item per metric, and an optional clock with a calendar and world clocks.
- Panel with Overview, Apps, Network, History, Dev and Storage tabs, detail screens for CPU, GPU,
  memory, disk, network, battery and sensors, and a detached window mode.
- Apps tab: per-app CPU, memory, disk and energy, Quit and Force Quit, signals, process details,
  a description of what each app or system process is.
- Network inspector: per-app rates, connections, listening ports and hosts; public IP, ping and
  speed test on request.
- History for up to a year with CSV export.
- Alerts as macOS notifications with a detail screen for each alert, and custom per-app rules.
- Storage map of a folder and a reviewed cleanup of eleven well-known cache locations that moves
  items to the Trash.
- Dev tab: local servers grouped by project, AI agent sessions and Docker containers.
- Ten themes with light and dark variants, twelve languages.
- `mac-pulse://` links, `mac-pulse -json`, and an MCP server (`mac-pulse mcp`).

[Unreleased]: https://github.com/kl09/mac-pulse/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/kl09/mac-pulse/releases/tag/v1.0.0
