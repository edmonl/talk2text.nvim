# Use socket-path rechecks for stale target cleanup
Date: 2026-07-17

## Status
Superseded by [0003](0003-use-directory-locks-for-target-lifecycle.md)

## Context

A command can determine a target stale while another Neovim instance replaces that target. Deleting the originally observed pathname without checking again can remove the replacement.

Full cross-process locking would prevent that race but adds shared synchronization state and coordination between the command and plugin.

## Decision

Before deleting a stale target, the command re-reads its normalized first-line socket path. It deletes the target only if that value still matches the value it probed. If it differs, the command leaves it untouched and advances to the next fallback for the current invocation.

## Consequences

+ A replacement observed by the recheck is preserved.
+ The current invocation has a simple fallback rule and does not retry the replacement.
+ No lock or additional runtime state is required.
- A replacement written after the recheck and before deletion can still be removed.
- A changed target receives later transcripts rather than the current one.
