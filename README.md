## Overview

`talk2text.nvim` connects the `talk2text` speech-to-text daemon to Neovim.

The project has two parts:

1. A Neovim plugin that registers an editor as a transcript target and appends transcripts to the current buffer.
2. A `talk2text-nvim` command intended for `talk2text daemon -out-cmd`.

Text is sent to an explicitly selected Neovim instance when one is available. Otherwise, it can reuse or start a dedicated default editor. Target files are updated and cleaned up under a shared runtime-directory lock so participating Neovim instances and output commands do not remove one another's targets.

## Quick Start

Install the plugin and command as described in [Installation](#installation), then add a mapping to your Neovim configuration:

```lua
require("talk2text").setup()

vim.keymap.set("n", "<leader>/", function()
  require("talk2text").set_target()
end, { desc = "Use this Neovim for talk2text" })
```

Start `talk2text` with the integration as its output command:

```sh
talk2text daemon -out-cmd talk2text-nvim
```

The daemon creates the runtime directory required by the integration. In Neovim, press the configured mapping before recording. Completed text transcripts are appended to the current buffer and removed after a successful load.

No keymap is defined by default. You can also select the current instance directly:

```vim
:lua require("talk2text").set_target()
```

## Installation

### Requirements

The initial supported environment is:

1. Linux.
2. Bash 4 or newer.
3. Neovim 0.9 or newer with LuaJIT.
4. The `talk2text` daemon.
5. The `flock` command, normally provided by `util-linux`.

The default client hook invokes `nvim`, so it must be on `PATH`. Desktop notifications use `notify-send` by default but are best-effort and optional.

### Install the Neovim Plugin

Add the repository root to Neovim's runtime path with your plugin manager. For example, with `lazy.nvim` and a local checkout:

```lua
{
  dir = vim.fn.expand("~/projects/talk2text.nvim"),
  config = function()
    require("talk2text").setup()
  end,
}
```

Without a plugin manager, link the checkout into Neovim's native package directory:

```sh
mkdir -p ~/.local/share/nvim/site/pack/talk2text/start
ln -s "$PWD" ~/.local/share/nvim/site/pack/talk2text/start/talk2text.nvim
```

Run that command from the repository root.

### Install the Output Command

Install the command somewhere on `PATH`:

```sh
install -Dm755 bin/talk2text-nvim ~/.local/bin/talk2text-nvim
```

Make sure `~/.local/bin` is on `PATH`, or pass the full command path to `talk2text daemon -out-cmd`. Rerun the install command after updating the checkout.

## Target Selection

Calling `set_target()` makes the current Neovim instance the explicit target. The plugin starts a Neovim server when necessary, records its socket in the shared runtime directory, and removes that target when the instance exits if it still belongs to the same server.

For each text transcript, `talk2text-nvim` tries targets in this order:

1. The explicitly selected `nvim-target`.
2. An existing `default-nvim-target`.
3. A newly started default editor.

Missing targets are skipped. Empty and unreachable target files are cleaned up before the command continues to the next choice. A reachable target whose transcript load fails is not treated as stale; delivery stops so the same transcript is not appended twice.

A `short` transcript acts as a shortcut to remove the explicit target. Future text then goes to the default editor. A `blank` transcript does not change either target.

## Default Editor

Starting a default editor requires a terminal hook because the distributed command does not assume a terminal emulator. For example, with Alacritty:

```sh
export TALK2TEXT_NVIM_TERMINAL_CMD='exec alacritty --class talk2text-editor --title talk2text --command nvim "$@"'
```

The default editor opens a no-file buffer, loads the transcript, and registers itself for later transcripts. In that buffer, `qq` copies the full buffer to the `+` clipboard register and quits without saving. If copying fails, the editor remains open.

When an existing default editor is reused, an optional focus hook can bring its window forward. A Sway configuration is available in [docs/examples/sway-alacritty.md](docs/examples/sway-alacritty.md).

## Command Hooks

The output command is configured through environment variables:

1. `TALK2TEXT_NVIM_CLIENT_CMD` controls Neovim server probes and remote loads. Its default is `nvim "$@"`.
2. `TALK2TEXT_NVIM_TERMINAL_CMD` starts a default editor. It is empty by default and is required only when no usable target exists.
3. `TALK2TEXT_NVIM_FOCUS_CMD` focuses an existing default editor. It is empty by default and is best-effort.
4. `TALK2TEXT_NVIM_NOTIFY_CMD` reports blank and short transcripts. Its default is `notify-send -t 5000 Talk2text "$@"` and is best-effort.

Each non-empty value is trusted shell code executed with `sh -c`. Generated values are passed as positional arguments and are available through `"$@"`; do not interpolate runtime values into hook code. The terminal hook runs with the `talk2text` runtime directory as its working directory. The focus hook receives no generated arguments.

## Runtime Directory

The plugin follows the same runtime-directory discovery order as `talk2text`:

1. `$XDG_RUNTIME_DIR/talk2text`.
2. `$TMPDIR/run-<uid>/talk2text`.
3. `/tmp/run-<uid>/talk2text`.

The daemon must create this directory before the plugin registers a target. To use an explicit directory, configure both `talk2text` and the plugin consistently:

```lua
require("talk2text").setup({
  runtime_dir = "/path/to/runtime",
})
```

The integration stores two target files there:

```text
nvim-target
default-nvim-target
```

These files contain Neovim server socket paths. Plugin writes and command reads and deletions use a shared exclusive advisory lock on the runtime directory.

## Lua API

The public module provides:

1. `setup(opts)` to set an optional `runtime_dir`.
2. `set_target()` to make the current Neovim instance the explicit target.
3. `set_default_target()` to make it the default editor target.
4. `load(path)` to append a transcript file to the current buffer and remove it after success.
5. `load()` to retry the most recent failed load remembered by the current Neovim session.

`load(path)` preserves intentional blank lines and appends complete lines without merging the first transcript line into existing buffer content. It returns `true` on success or `nil, err` on failure.

## Troubleshooting

1. If `set_target()` reports that the runtime directory is unavailable, start the `talk2text` daemon first and confirm that the plugin and daemon resolve the same runtime directory.
2. If transcripts do not reach Neovim, confirm that `talk2text-nvim` is on `PATH`, `nvim` is on `PATH`, and the selected Neovim instance is still running.
3. If no selected target exists and default-editor startup fails, configure `TALK2TEXT_NVIM_TERMINAL_CMD`.
4. If an existing default editor does not come forward, configure `TALK2TEXT_NVIM_FOCUS_CMD` for your window manager.
5. If `qq` cannot copy from the default editor, check Neovim's `+` clipboard provider with `:checkhealth`.
6. Run `talk2text-nvim <kind> <absolute-transcript-path>` in a terminal to inspect errors written to stderr.

More detailed behavior is documented in [docs/spec.md](docs/spec.md).

## Contributing

Run the test suite from the repository root:

```sh
tests/run.sh
```

The tests start local headless Neovim servers and create temporary Unix sockets under `/tmp`.
