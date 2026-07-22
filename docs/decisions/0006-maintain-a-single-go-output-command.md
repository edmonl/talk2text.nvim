# Maintain a single Go output command
Date: 2026-07-22

## Status
Accepted

## Context
The repository carried two output-command implementations with the same public behavior. Maintaining both duplicated fixes and tests and allowed their RPC, locking, and process behavior to diverge.

## Decision
Maintain one production output command implemented in Go. Continue to specify the command's public behavior independently of its implementation language.

## Consequences
+ Command fixes and automated scenarios target one maintained implementation.
+ The behavioral specification remains usable if the implementation changes in the future.
- Building the output command from source requires a supported Go toolchain.
