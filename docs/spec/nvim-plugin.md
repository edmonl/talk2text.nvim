# Neovim Plugin

The plugin uses a normal plugin-manager layout with Lua module files under `lua/talk2text/`. A `plugin/*.lua` loader is not required for the initial implementation.

The public Lua API is:

```lua
require("talk2text").setup(opts)
require("talk2text").set_target(id)
require("talk2text").load(id)
```

# `setup(opts)`

Configures the plugin. Supported options:

1. `runtime_dir`: optional explicit runtime directory.

Example:

```lua
require("talk2text").setup({
  runtime_dir = "/path/to/runtime",
})
```

`setup()` resolves and validates the runtime directory, then confirms that `<runtime_dir>/daemon.sock` accepts a connection. It does not perform a daemon status request. The first runtime directory successfully resolved by `setup()`, `set_target()`, or `load()` remains fixed for the Neovim session. A later `setup()` call for the same directory succeeds without checking the daemon again, but it cannot switch to another directory. On failure, `setup()` emits one error notification inside Neovim and returns `nil, err` without raising another error. Calling `setup()` does not make the current Neovim instance the target. Plugin configuration comes from the user's normal Neovim configuration; the output command does not call `setup()`.

# `set_target([id])`

Makes the current Neovim instance the `talk2text` target, then applies the same ID behavior as `load([id])`.

Behavior:

1. Resolves the runtime directory from `setup({ runtime_dir = ... })` or from the same environment-based discovery rules as `talk2text`.
2. Fails with an error if the runtime directory is missing, invalid, or unavailable.
3. Starts a Neovim server if the current instance does not already have one.
4. Writes the current server socket path to `<runtime_dir>/nvim-target`.
5. Registers quit-time cleanup that deletes `nvim-target` only when it still points to this same server socket.
6. Emits an informational notification only when the target actually changes to this server. Repeating `set_target()` in the same target emits no target-switch notification.
7. Calls `load(id)` after target registration. Target registration happens even when `id` is invalid, so a negative or otherwise invalid ID reports a load error without undoing the switch.
8. On target-registration failure, emits one error notification inside Neovim and returns `nil, err` without raising another error.

Normal Neovim sessions do not become the target unless the user explicitly calls `set_target()`. There is no default keymap. Users may define their own, for example:

```lua
vim.keymap.set("n", "<leader>/", function()
  require("talk2text").set_target(vim.v.count)
end)
```

With this mapping, a positive count selects the editor and loads that transcript ID. No count supplies `0`, which selects the editor and retries the last failed load if one exists.

# `load([id])`

Loads a transcript into the current buffer by its runtime-scoped clip ID.

Behavior:

1. A positive integer `id` reads `<runtime_dir>/transcripts/<id>.txt` using the runtime configured by `setup()` or the normal discovery rules.
2. `load()`, `load(nil)`, and `load(0)` retry the remembered failed ID. If no ID is remembered, they are successful no-ops.
3. A negative, fractional, or nonnumeric ID is invalid. It emits an error notification that includes the supplied ID and returns `nil, err`.
4. Leading and trailing transcript whitespace is discarded. An empty transcript is a no-op.
5. A non-empty transcript is treated as a single word when it contains no whitespace and its final character does not match the Lua punctuation class `%p`. Internal punctuation is allowed, so `well-known` is a word but `unfinished-` is not.
6. A single-word transcript is inserted on the cursor's current line without splitting the current whitespace-delimited word. If the cursor is within a word, insertion happens after the whole word.
7. Insertion happens before trailing punctuation under the cursor. Punctuation is trailing when only punctuation appears between the cursor and the next whitespace or end of line; punctuation followed by more non-whitespace text remains part of the current word, as in `it's`.
8. When surrounding text exists, the inserted word is preceded by existing whitespace or at least one added space and followed by at least one whitespace character or punctuation. Existing whitespace before the cursor is preserved.
9. After single-word insertion, the plugin attempts to move the cursor to the beginning of the inserted word. Failure to move the cursor does not fail an otherwise successful load.
10. Other non-empty transcripts are appended to the buffer's last line. An empty last line is replaced directly; otherwise, trailing spaces are normalized and exactly one space separates the existing line from the transcript's first line. Remaining transcript lines are appended as separate lines, preserving interior blank lines.
11. It removes the source file only after the load or no-op succeeds.
12. Returns `true` on a successful load or no-op.
13. Returns `nil, err` on expected failure.
14. On failure, emits one error notification inside Neovim and returns `nil, err` without raising another error. Transcript-specific notifications include the ID. This applies equally to direct Lua calls, remote command delivery, and default-editor startup.
15. After a remembered failed ID is retried and loaded successfully, emits an informational notification that includes the ID. A retry with no remembered ID remains a silent successful no-op.

The plugin remembers one failed ID in memory for the current Neovim session. A failed explicit load replaces the remembered ID, a failed retry keeps it, and any successful load clears it. A failed load does not partially change the buffer. If removing the source file fails after text was appended, the plugin returns `nil, err` without remembering a retry, because retrying could append the same transcript twice.

# Default Editor Startup

When the command starts a default editor, startup should:

1. Use the user's normal Neovim configuration, including any user-provided `require("talk2text").setup(...)` call.
2. Register the new Neovim server as `default-nvim-target` through the internal startup adapter.
3. For `text <path>`, load the derived transcript ID through the internal startup adapter.
4. Configure the `qq` normal-mode mapping for the default editor buffer.

If default-target registration fails, report the error in Neovim and still attempt the initial load. A successful load still removes the transcript even when registration failed.

If the startup load fails, report the error in Neovim and retain the transcript for retry.

The default editor uses a no-file buffer. It should not open the transcript file as the editable buffer.

The buffer-local `qq` mapping copies the full default editor buffer content to the `+` clipboard register. If copying fails, it leaves the window open. After a successful copy, it closes the current window; Neovim exits when that is its last window. Because the transcript buffer has `buftype=nofile`, its modifications do not block closing. Its `bufhidden=wipe` setting removes the transcript buffer after its last window closes. The mapping does not force-close other windows or modified normal buffers. It does not save anything or emit output.

Further transcripts are loaded into the current buffer of the default editor while that Neovim instance remains the usable default target and no explicit target overrides it. If the user changes the current buffer, later transcripts are loaded into that buffer. If the explicit target changes, the default editor exits, or either target becomes stale, future transcripts follow the target resolution order described in the main spec.
