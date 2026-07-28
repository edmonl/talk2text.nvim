# Neovim Plugin

The public Lua API is:

```lua
require("talk2text").setup(opts)
require("talk2text").set_target(id)
require("talk2text").load(id)
require("talk2text").start_default_target(id)
```

`setup()`, `set_target()`, and `load()` return `true` on success or `false, err` on expected failure. They report failures through Neovim notifications rather than raising another error.

# `setup(opts)`

Configures the plugin. Supported options:

1. `runtime_dir`: optional explicit runtime directory.

Example:

```lua
require("talk2text").setup({
  runtime_dir = "/path/to/runtime",
})
```

`setup()` selects and validates the runtime directory and requires the daemon to be available. The first successfully selected runtime remains fixed for the Neovim session; selecting it again succeeds, while switching to another runtime fails. Calling `setup()` does not make the current Neovim instance the target.

# `set_target([id])`

Makes the current Neovim instance the `talk2text` target, then applies the same ID behavior as `load([id])`.

It resolves the runtime directory, ensures the current instance has a Neovim server, and publishes that server as `nvim-target`. The target is removed on exit only if it still belongs to this instance. An informational notification is emitted only when the target changes.

Target registration happens before `load(id)`. A load failure, including an invalid ID, does not undo a successful target switch.

Normal Neovim sessions do not become the target unless the user explicitly calls `set_target()`. There is no default keymap. Users may define their own, for example:

```lua
vim.keymap.set("n", "<leader>/", function()
  require("talk2text").set_target(vim.v.count)
end)
```

With this mapping, a positive count selects the editor and loads that transcript ID. No count supplies `0`, which selects the editor and retries the last failed load if one exists.

# `load([id])`

Loads a transcript into the current buffer by its runtime-scoped clip ID.

1. A positive safe integer ID reads `<runtime_dir>/transcripts/<id>`. Other values are invalid, except `nil` and `0`, which retry the remembered failed ID or succeed as no-ops when none exists.
2. Leading and trailing transcript whitespace is discarded. An empty transcript does not change the buffer.
3. A transcript with no whitespace and no trailing punctuation is inserted as a word after the word under the cursor and before its trailing punctuation. Existing whitespace is preserved where possible, and the cursor moves to the inserted word.
4. Other non-empty transcripts are appended at the current line. Existing trailing spaces are normalized, interior transcript lines are preserved, and the cursor moves to the final inserted line.
5. The source file is removed only after a successful load or no-op.
6. Transcript failures notify with the ID. A successful retry emits an informational notification; a retry with no remembered failure is silent.

The plugin remembers one failed ID in memory for the current Neovim session. A failed explicit load replaces the remembered ID, a failed retry keeps it, and any successful load clears it. A failed load does not partially change the buffer. If removing the source file fails after text was inserted, the plugin returns `false, err` without remembering a retry, because retrying could insert the same transcript twice.

# `start_default_target(id)`

Configures the current Neovim instance as a plugin-managed default target. It publishes `default-nvim-target`, loads the supplied transcript, makes the buffer disposable, and adds a buffer-local `qq` mapping.

The output command does not call this function automatically when launching the configured default editor. Custom launch integrations may invoke it when they want this behavior.

If target registration fails, the initial load is still attempted. If loading fails, the transcript remains available for retry.

The `qq` mapping copies the full buffer to the `+` clipboard and closes its window. A copy failure leaves the window open. Other windows and modified normal buffers are not force-closed.

Further transcripts are loaded into the current buffer of the default editor while that Neovim instance remains the usable default target and no explicit target overrides it. If the user changes the current buffer, later transcripts are loaded into that buffer. If the explicit target changes, the default editor exits, or either target becomes stale, future transcripts follow the target resolution order described in the main spec.
