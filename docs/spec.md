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

During automatic discovery, missing candidates are skipped and the first valid directory is selected. An invalid or unavailable candidate stops discovery. The plugin may instead use an explicit runtime directory, which is validated without fallback. Once selected, the runtime directory remains fixed for the Neovim session.

The command derives the runtime directory and transcript ID from the absolute transcript path supplied by `talk2text`. The path must be `<runtime_dir>/transcripts/<positive-id>` and the runtime directory must not be the filesystem root. Path validation uses the supplied path without resolving symlinks. See the [command spec](spec/cli.md) for output-kind behavior.

This integration owns transcript cleanup. Delivery to an existing target removes text only after success; a newly launched editor owns the supplied path. Blank and short transcripts are removed before their kind-specific behavior, and cleanup failure is best-effort.

Neither the plugin nor the command creates the runtime directory. A missing explicit runtime directory or an invalid or unavailable candidate is an error. When automatic discovery finds no existing candidate, `setup()` succeeds silently so configurations can load on machines without the service; runtime-dependent API calls still report the missing service as an error. The `talk2text` daemon is expected to create and own the runtime directory before this integration is used.

# Target Files

There are two Neovim target files:

```text
<runtime_dir>/nvim-target
<runtime_dir>/default-nvim-target
```

Each target identifies one absolute Neovim server socket path. `nvim-target` is the explicit user-selected target; `default-nvim-target` identifies a reusable default editor.

A missing or zero-byte target is absent. A malformed or non-absolute target is removed when safe to do so and causes the current delivery to fail.

An unreachable target is stale: it is removed when safe, an informational notification is emitted when removal occurs, and delivery falls back. A reachable target that rejects a transcript is retained and causes delivery to fail.

When the command needs to send text to Neovim, it resolves targets in this order:

1. Try `nvim-target`.
2. If `nvim-target` is missing, zero-byte, or stale, try `default-nvim-target`.
3. If `default-nvim-target` is missing, zero-byte, or stale, start a new default Neovim editor.

Target publication, replacement, and cleanup are concurrency-safe. A stale check or exiting Neovim instance must not remove a target that another instance has replaced. See [ADR 0003](decisions/0003-use-directory-locks-for-target-lifecycle.md).

# Future Considerations

1. Concurrent output-command invocations may start separate default editors. Each editor remains usable, while the later target-file write determines the editor reused for future transcripts. Revisit synchronization only if this becomes a practical user-experience problem.
