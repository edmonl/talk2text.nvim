# Delegate default-editor transcript cleanup to Neovim
Date: 2026-07-17

## Status
Accepted

Supersedes [0001](0001-defer-output-command-startup-timeouts.md)

## Context

`talk2text` leaves transcript cleanup to its output command and permits that command to delegate processing. A terminal-started Neovim can outlive the command that invokes it, so terminal process completion cannot prove that the initial transcript was loaded.

## Decision

For a newly started default editor, delegate transcript loading and cleanup to the launched Neovim instance. It registers the default target, loads the transcript, and removes the transcript only after a successful load. The invoking command does not use target registration or initial load as its own completion condition.

## Consequences

+ Terminal hooks can be interactive or long-running without becoming a delivery-coordination protocol.
+ The transcript remains available until Neovim consumes it.
+ Failed initial loads retain the transcript for retry or daemon startup cleanup.
- Initial-load errors are reported by Neovim rather than returned to the invoking command.
