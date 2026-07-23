# Use runtime-scoped clip IDs in the plugin API
Date: 2026-07-19

## Status
Accepted

## Context

The output command receives a transcript path from `talk2text`, while the plugin already owns runtime-directory configuration. Passing arbitrary paths through the public API and internal RPC boundary duplicates path construction and requires escaping paths in both command implementations.

## Decision

Use positive clip IDs at the Lua API boundary. The output command continues to accept the daemon-provided path, validates a canonical `<positive-id>` filename, derives the runtime directory and ID, and passes only the ID to Neovim. The plugin constructs `<configured-runtime>/transcripts/<id>`. Internal remote-load and default-start adapters also receive only the ID.

`nil` and `0` mean retry the last failed ID. Invalid IDs are reported inside Neovim. `set_target(id)` claims the target before delegating to `load(id)`, so an invalid ID does not undo a successful target switch.

## Consequences

+ Public calls are consistent with clip identity rather than filesystem location.
+ RPC and startup adapters no longer need path escaping or an additional runtime argument.
+ Transcript-specific notifications can identify the clip directly.
- The plugin is coupled to `talk2text`'s transcript naming convention and no longer loads arbitrary paths.
- The plugin and daemon must resolve the same runtime directory because transcript IDs are not globally unique.
