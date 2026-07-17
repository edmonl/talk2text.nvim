# Neovim Plugin

The plugin uses a normal plugin-manager layout with Lua module files under `lua/talk2text/`. A `plugin/*.lua` loader is not required for the initial implementation.

The public Lua API is:

```lua
require("talk2text").setup(opts)
require("talk2text").set_target()
require("talk2text").set_default_target()
require("talk2text").load(path)
require("talk2text").load()
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

Calling `setup()` does not make the current Neovim instance the target. Plugin configuration comes from the user's normal Neovim configuration; the output command does not call `setup()`.

# `set_target()`

Makes the current Neovim instance the `talk2text` target.

Behavior:

1. Resolves the runtime directory from `setup({ runtime_dir = ... })` or from the same environment-based discovery rules as `talk2text`.
2. Fails with an error if the runtime directory is missing, invalid, or unavailable.
3. Starts a Neovim server if the current instance does not already have one.
4. Writes the current server socket path to `<runtime_dir>/nvim-target`.
5. Registers quit-time cleanup that deletes `nvim-target` only when it still points to this same server socket.

Normal Neovim sessions do not become the target unless the user explicitly calls `set_target()`. There is no default keymap. Users may define their own, for example:

```lua
vim.keymap.set("n", "<leader>/", function()
  require("talk2text").set_target()
end)
```

# `set_default_target()`

Makes the current Neovim instance the default editor target.

Behavior:

1. Resolves the runtime directory from `setup({ runtime_dir = ... })` or from the same environment-based discovery rules as `talk2text`.
2. Fails with an error if the runtime directory is missing, invalid, or unavailable.
3. Starts a Neovim server if the current instance does not already have one.
4. Writes the current server socket path to `<runtime_dir>/default-nvim-target`.
5. Registers quit-time cleanup that deletes `default-nvim-target` only when it still points to this same server socket.

Command-started default editor startup uses `set_default_target()`. Normal user-selected Neovim sessions use `set_target()`.

# `load([path])`

Loads a transcript file into the current buffer. Without `path`, it retries the last failed load.

Initial behavior:

1. `load(path)` reads `path`.
2. `load()` retries the remembered failed path. If no path is remembered, it is a no-op.
3. An empty transcript is a no-op. A non-empty transcript is appended as complete lines at the end of the current buffer. A buffer containing only its initial empty line is treated as empty. Intentional blank lines are preserved, but a final source newline does not add a blank line.
4. It removes the source file only after the load or no-op succeeds.
5. Returns `true` on a successful load or no-op.
6. Returns `nil, err` on failure.

The plugin remembers one failed path in memory for the current Neovim session. A failed explicit load replaces the remembered path, a failed retry keeps it, and any successful load clears it. A failed load does not partially change the buffer. If removing the source file fails after text was appended, the plugin returns `nil, err` without remembering a retry, because retrying could append the same transcript twice.

The append strategy may become more sophisticated later. For now, it should avoid merging the first appended line into the previous final line when the buffer already has content.

# Default Editor Startup

When the command starts a default editor, startup should:

1. Use the user's normal Neovim configuration, including any user-provided `require("talk2text").setup(...)` call.
2. Call `require("talk2text").set_default_target()` from startup commands.
3. For `text <path>`, call `require("talk2text").load(path)` from startup commands.
4. Configure the `qq` normal-mode mapping for the default editor buffer.

If default-target registration fails, report the error in Neovim and still attempt the initial load. A successful load still removes the transcript even when registration failed.

If the startup load fails, report the error in Neovim and retain the transcript for retry.

The default editor uses a no-file buffer. It should not open the transcript file as the editable buffer.

The buffer-local `qq` mapping copies the full default editor buffer content to the `+` clipboard register. If copying fails, it leaves the editor open. After a successful copy, it quits Neovim without a save prompt; modifications to the default editor's transcript buffer do not block that quit. It does not save anything, clear the buffer, emit output, or preserve state after quit.

Further transcripts are loaded into the current buffer of the default editor while that Neovim instance remains the usable default target and no explicit target overrides it. If the user changes the current buffer, later transcripts are loaded into that buffer. If the explicit target changes, the default editor exits, or either target becomes stale, future transcripts follow the target resolution order described in the main spec.
