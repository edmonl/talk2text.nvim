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

`talk2text` owns the runtime directory. This project uses the same runtime directory discovery rules as `talk2text`:

1. `$XDG_RUNTIME_DIR/talk2text` when `XDG_RUNTIME_DIR` is set.
2. Otherwise `$TMPDIR/run-<uid>/talk2text` when `TMPDIR` is set.
3. Otherwise `/tmp/run-<uid>/talk2text`.

The plugin may also be configured with an explicit runtime directory.

For every kind, the command validates the literal transcript path it receives from `talk2text`. The path must be absolute and its immediate parent directory must be `transcripts`; that directory's parent is the runtime directory. The inferred runtime directory must not be the filesystem root. The command does not resolve symlinks or normalize path components. `text` requires a readable regular file. `blank` and `short` also accept an already-absent file: they have no transcript content to read, and cleanup is complete once the file is absent. Any other path shape is an error.

As `talk2text`'s output command, this project owns cleanup of the supplied transcript file. For `text`, it removes the file only after successful handling and otherwise leaves it for `talk2text`'s next startup cleanup. For `blank` and `short`, it attempts to remove the file before other kind-specific handling. Cleanup failure is reported but does not interrupt that handling or by itself make the command fail.

Neither the plugin nor the command creates the runtime directory. If the runtime directory is missing, invalid, or unavailable, that is an error. The `talk2text` daemon is expected to create and own the runtime directory before this integration is used.

# Target Files

There are two Neovim target files:

```text
<runtime_dir>/nvim-target
<runtime_dir>/default-nvim-target
```

Each file contains only one value: a Neovim server socket path.

Readers use the first line of the target file as the socket path and ignore later lines. Leading and trailing whitespace around the first line is ignored. If the first line is empty or does not identify a usable Neovim server socket, the target file is treated as unusable.

`nvim-target` is the explicit current target. Normal Neovim instances write this file when the user explicitly makes that instance the target.

`default-nvim-target` is the default editor target. A Neovim instance launched by the command writes this file when it starts.

When the command needs to send text to Neovim, it resolves targets in this order:

1. Try `nvim-target`.
2. If `nvim-target` is missing, empty, stale, or unusable, try `default-nvim-target`.
3. If `default-nvim-target` is missing, empty, stale, or unusable, start a new default Neovim editor.

The plugin and command use a shared exclusive advisory lock on the runtime directory for every target-file read, write, and deletion. The lock serializes only participating integration processes; it does not affect ordinary runtime-directory operations.

When checking a target, the command reads its normalized first-line value while holding the lock, then releases the lock before probing the Neovim socket. If the target is stale or unusable, the command reacquires the lock and re-reads the value before deleting the file. It deletes the file only when the value still matches the value it probed. If the value changed, the command leaves the file untouched and continues to the next fallback; the changed target is considered only by a later command invocation.

When a Neovim instance becomes a target, the plugin acquires the same lock and overwrites the relevant target file with that instance's server socket path. The write should replace the target file atomically where practical by writing a temporary sibling file and renaming it over the target file. On exit, the plugin acquires the same lock and deletes that target file only if the file still contains its own socket path. See [ADR 0003](decisions/0003-use-directory-locks-for-target-lifecycle.md).

# Future Considerations

1. Concurrent output-command invocations may start separate default editors. Each editor remains usable, while the later target-file write determines the editor reused for future transcripts. Revisit synchronization only if this becomes a practical user-experience problem.
