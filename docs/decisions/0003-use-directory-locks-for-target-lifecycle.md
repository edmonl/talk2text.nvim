# Use directory locks for target lifecycle
Date: 2026-07-17

## Status
Accepted

Supersedes [0002](0002-use-socket-path-rechecks-for-stale-target-cleanup.md)

## Context

A socket-path recheck prevents a stale cleanup from removing a replacement observed before deletion, but a replacement can still occur between the recheck and deletion.

The command and plugin are the participating writers of target files. The runtime directory is stable while the daemon runs, and an advisory lock on it does not interfere with ordinary directory operations.

## Decision

The command and plugin use a shared exclusive advisory lock on the runtime directory for target-file lifecycle operations. The command reads a target under the lock, releases it to probe the socket, then reacquires it to recheck the socket path and conditionally delete a stale target. Target writes retain atomic replacement and use the same lock.

## Consequences

+ A participating writer cannot replace a target between its final recheck and deletion.
+ Slow socket probes and editor startup do not hold the lock.
+ Ordinary runtime-directory operations remain unaffected.
- Every command and plugin implementation must honor the shared lock.
- External target-file changes that do not use the lock can still race with the integration.
