# Goal

`talk2text.nvim` is the Neovim integration for the `talk2text` ecosystem.

This project has two parts:

1. A Neovim plugin.
2. An executable command named `talk2text-nvim`.

The command is intended to be used as the `talk2text daemon --out-cmd` command.

Detailed component specs:

1. [Plugin spec](spec/nvim-plugin.md)
2. [Command spec](spec/cli.md)

# Runtime Model

`talk2text` owns the runtime directory. The plugin checks the same runtime directory candidates as `talk2text`, in this order:

1. `$XDG_RUNTIME_DIR/talk2text` when `XDG_RUNTIME_DIR` is set.
2. `$TMPDIR/run-<uid>/talk2text` when `TMPDIR` is set.
3. `/tmp/run-<uid>/talk2text`.

During automatic discovery, a missing candidate is skipped and the next candidate is checked. The first existing directory is selected. An existing candidate that is not a directory, or a candidate that cannot be inspected for a reason other than being absent, is an error and stops discovery. The plugin caches the first successfully resolved directory for the Neovim session; a failed resolution is not cached and may be retried. The plugin may instead be configured with an explicit runtime directory; an explicit path is validated directly, cached after successful setup, and never falls back to another candidate.

For every kind, the command validates the transcript path it receives from `talk2text`. The path must be absolute, its immediate parent directory must be `transcripts`, and its filename must be the canonical form `<positive-id>`; that directory's parent is the runtime directory. The inferred runtime directory must not be the filesystem root. The command does not resolve symlinks before validating this shape: a symlink path with valid visible components is accepted without checking the resolved target's path shape. The command routes `text` transcripts without opening them; Neovim is responsible for reading them. `blank` and `short` also accept an already-absent file: they have no transcript content to read, and cleanup is complete once the file is absent. Any other path shape is an error.

The plugin resolves transcript IDs relative to its configured or discovered runtime directory. The command passes only the derived ID to the plugin's internal adapters. The project assumes a user runs one daemon under the default runtime resolution; supporting multiple same-user daemons under that same default is outside the expected runtime model. Tests provide the runtime directory explicitly.

As `talk2text`'s output command, this project owns cleanup of the supplied transcript file. For `text`, it removes the file only after successful handling and otherwise leaves it for `talk2text`'s next startup cleanup. For `blank` and `short`, it attempts to remove the file before other kind-specific handling. Cleanup failure is reported but does not interrupt that handling or by itself make the command fail.

Neither the plugin nor the command creates the runtime directory. A missing explicit runtime directory, no existing automatic-discovery candidate, or an invalid or unavailable candidate is an error. The `talk2text` daemon is expected to create and own the runtime directory before this integration is used.

# Target Files

There are two Neovim target files:

```text
<runtime_dir>/nvim-target
<runtime_dir>/default-nvim-target
```

Each file contains only one value: an absolute Neovim server socket path.

Readers use the first line of the target file as the socket path and ignore later lines. Leading and trailing whitespace around the first line is ignored. A missing or zero-byte target is treated as absent. A nonempty target whose first line is blank after trimming, cannot be read completely, or cannot be closed is malformed. The command deletes a malformed target while retaining the runtime lock, reports a notification beginning with `Error: `, and stops delivery with a nonzero status. If malformed-target deletion also fails, that cleanup failure is included in the fatal error.

After reading a nonempty socket path, the command requires it to be absolute. A non-absolute value is invalid: the command conditionally deletes the target if its value has not changed, reports an `Error: ` notification, and stops delivery. An absolute path is tested by connecting to and probing the Neovim server rather than by separately inspecting the socket file. If connection or probing fails, the target is stale. The command conditionally deletes an unchanged stale target, reports a stale-target notification only when deletion occurs, and continues to the next fallback. If a reachable target fails to load the transcript, the target is retained and delivery stops with an `Error: ` notification.

`nvim-target` is the explicit current target. Normal Neovim instances write this file when the user explicitly makes that instance the target.

`default-nvim-target` is the default editor target. A Neovim instance launched by the command writes this file when it starts.

When the command needs to send text to Neovim, it resolves targets in this order:

1. Try `nvim-target`.
2. If `nvim-target` is missing, zero-byte, or stale, try `default-nvim-target`.
3. If `default-nvim-target` is missing, zero-byte, or stale, start a new default Neovim editor.

Malformed and invalid targets are fatal and do not trigger fallback.

The plugin and command use a shared exclusive advisory lock on the runtime directory for every target-file read, write, and deletion. The lock serializes only participating integration processes; it does not affect ordinary runtime-directory operations.

When checking a target, the command reads its normalized first-line value while holding the lock. Read and format errors are cleaned up immediately under that lock. The command releases the lock before validating and probing a nonempty socket path. Before deleting an invalid or stale target, it reacquires the lock and re-reads the value. It deletes the file only when the value still matches the value it validated or probed. If the value changed, the command leaves it untouched. An invalid observed value remains fatal; a changed target after a stale probe is considered only by a later command invocation while the current invocation continues to the next fallback.

When a Neovim instance becomes a target, the plugin acquires the same lock and overwrites the relevant target file with that instance's server socket path. The write should replace the target file atomically where practical by writing a temporary sibling file and renaming it over the target file. On exit, the plugin acquires the same lock and deletes that target file only if the file still contains its own socket path. See [ADR 0003](decisions/0003-use-directory-locks-for-target-lifecycle.md).

# Future Considerations

1. Concurrent output-command invocations may start separate default editors. Each editor remains usable, while the later target-file write determines the editor reused for future transcripts. Revisit synchronization only if this becomes a practical user-experience problem.
