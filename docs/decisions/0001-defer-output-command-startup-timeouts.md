# Defer output-command startup timeouts
Date: 2026-07-17

## Status
Superseded by [0004](0004-delegate-default-editor-transcript-cleanup-to-neovim.md)

## Context

A new default editor is ready only after it registers a target and loads the initial transcript. A failed or hung launch can leave its output command running.

The companion daemon waits for an output command to exit before cleaning up that clip's transcript. That wait is confined to the completed clip's processing goroutine: capture state is released and shortcut-facing requests are acknowledged before transcript processing begins. The daemon continues to accept and acknowledge later requests, and can record and process later clips while an earlier output command is still running. Only cleanup for the affected clip waits.

## Decision

The initial implementation does not impose a timeout, retry, or cancellation policy on default-editor startup. Target registration and the initial load remain the success condition.

## Consequences

+ A normal successful startup exits after the editor is ready.
+ A hung output command does not prevent later shortcut requests, recording, or processing of later clips.
- A hung startup keeps its output command and transcript cleanup pending until it exits or is terminated externally.
- Resource or observability concerns may require a superseding decision later.
