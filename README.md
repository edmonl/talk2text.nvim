## Overview

`talk2text.nvim` connects the `talk2text` speech-to-text daemon to Neovim.

The project has two parts:

1. A Neovim plugin that registers an editor as a transcript target and inserts transcripts into the current buffer.
2. A `talk2text-nvim` command intended for `talk2text daemon -out-cmd`.

Text is sent to an explicitly selected Neovim instance when one is available. Otherwise, it can reuse or start a dedicated default editor. Target files are updated and cleaned up under a shared runtime-directory lock so participating Neovim instances and output commands do not remove one another's targets.

## Quick Start

Install the plugin and command as described in [Installation](#installation), then add a mapping to your Neovim configuration:

```lua
require("talk2text").setup()

vim.keymap.set("n", "<leader>/", function()
  require("talk2text").set_target(vim.v.count)
end, { desc = "Use this Neovim for talk2text" })
```

Start `talk2text` with the integration as its output command:

```sh
talk2text daemon -out-cmd talk2text-nvim
```

The daemon creates the runtime directory required by the integration. In Neovim, press the configured mapping before recording. Completed text transcripts are appended to the cursor's current line and removed after a successful load.

No keymap is defined by default. With the example mapping, a positive Vim count also loads that transcript ID; no count selects the editor and retries a previously failed load if one exists. You can also select the current instance directly:

```vim
:lua require("talk2text").set_target()
```

## Installation

### Requirements

The supported environment is:

1. Linux.
2. Neovim 0.10 or newer with LuaJIT.
3. The `talk2text` daemon.
4. A POSIX `sh` for configurable command hooks.

The Bash output command additionally requires Bash 4 or newer and the `flock` command, normally provided by `util-linux`. Building the Go output command requires Go 1.22 or newer; the resulting binary performs target locking and Neovim RPC directly.

The default editor command invokes `nvim`, so it must be on `PATH` when a new editor needs to be started. Desktop notifications use `notify-send` by default but are best-effort and optional.

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

The existing Bash implementation can be installed directly:

```sh
install -Dm755 bin/talk2text-nvim ~/.local/bin/talk2text-nvim
```

Alternatively, build and install the Go implementation from the project root:

```sh
mkdir -p ~/.local/bin
go build -o ~/.local/bin/talk2text-nvim .
```

The Go implementation connects to existing targets through Neovim's MessagePack-RPC socket without starting a separate `nvim` client. It starts notification and focus hooks as detached subprocesses. For new-editor startup, it replaces itself with the configured shell command so that command's exit status propagates to the caller. The Bash implementation remains in `bin/` as a reference and fallback.

Make sure `~/.local/bin` is on `PATH`, or pass the full command path to `talk2text daemon -out-cmd`. Reinstall or rebuild the selected implementation after updating the checkout.

## Target Selection

Calling `set_target()` makes the current Neovim instance the explicit target. The plugin starts a Neovim server when necessary, records its socket in the shared runtime directory, and removes that target when the instance exits if it still belongs to the same server.

For each text transcript, `talk2text-nvim` tries targets in this order:

1. The explicitly selected `nvim-target`.
2. An existing `default-nvim-target`.
3. A newly started default editor.

Missing targets are skipped. Empty and unreachable target files are cleaned up before the command continues to the next choice. A reachable target whose transcript load fails is not treated as stale; delivery stops so the same transcript is not inserted twice.

A `short` transcript acts as a shortcut to remove the explicit target. Future text then goes to the default editor. A `blank` transcript does not change either target.

## Default Editor

By default, `talk2text-nvim` starts a new default editor by invoking the configured Neovim command directly. An optional launch command can prepare the new Neovim instance for a particular environment; the Neovim command and generated startup arguments are appended to it. If default-editor startup launches a graphical application, ensure that the output command has the graphical-session environment required by that application.

The default editor opens a no-file buffer, loads the transcript, and registers itself for later transcripts. In that buffer, `qq` copies the full buffer to the `+` clipboard register and closes the current window. The transcript buffer is wiped after its last window closes. Neovim exits when that is its last window; other windows and buffers are not forcefully closed. If copying fails, the window remains open.

When an existing default editor is reused, an optional focus hook can bring its window forward.

## Command Hooks

The output command is configured through environment variables:

1. `TALK2TEXT_NVIM_CMD` controls Neovim server probes and remote loads in the Bash implementation, and the Neovim process used for default-editor startup in both implementations. The Go implementation performs existing-target probes and loads directly over MessagePack-RPC. Its default is `nvim`.
2. `TALK2TEXT_NVIM_LAUNCH_CMD` optionally starts a default editor through a launch command. It is empty by default. When set, the Neovim command and generated arguments are appended to it; when unset, the Neovim command is invoked directly.
3. `TALK2TEXT_NVIM_FOCUS_CMD` focuses an existing default editor. It is empty by default and is best-effort.
4. `TALK2TEXT_NVIM_NOTIFY_CMD` reports blank and short transcripts. Its default is `notify-send -a talk2text -u normal -t 5000 Talk2text` and is best-effort.

Each non-empty value is trusted shell code executed with `sh -c`. Generated arguments are appended internally, so command settings do not include `"$@"`. Runtime values are passed as shell positional parameters and are never interpolated into hook code. Hooks inherit the output command's environment and working directory. Integrations are responsible for providing any environment required by configured hooks. The focus hook receives no generated arguments. The Go implementation writes detached notification and focus hook startup errors and hook stderr to the output command's stderr without changing its exit status.

## Runtime Directory

The plugin checks the same runtime-directory candidates as `talk2text`, in order:

1. `$XDG_RUNTIME_DIR/talk2text`.
2. `$TMPDIR/run-<uid>/talk2text`.
3. `/tmp/run-<uid>/talk2text`.

Missing candidates are skipped. The first existing directory is used; an existing non-directory or another inspection failure stops discovery with an error. A successfully selected directory is cached for the Neovim session, while a failed discovery can be retried. The daemon must create the selected directory and accept connections on `daemon.sock` before plugin setup. Setup uses the connection as a liveness check; it does not issue a status request. An explicitly configured directory is validated without fallback, so configure both `talk2text` and the plugin consistently:

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

1. `setup(opts)` to set an optional `runtime_dir` and confirm its daemon socket is reachable.
2. `set_target([id])` to make the current Neovim instance the explicit target, then apply the same ID behavior as `load([id])`.
3. `load(id)` to load `<runtime_dir>/transcripts/<id>.txt` into the current buffer and remove it after success.
4. `load()`, `load(nil)`, or `load(0)` to retry the most recent failed ID remembered by the current Neovim session.

Lua API IDs must be integers from 1 through 9007199254740991 when supplied explicitly. `load(id)` trims leading and trailing transcript whitespace. A transcript with no whitespace and no punctuation at its end is inserted relative to the cursor: it does not split the current whitespace-delimited word, it is placed before trailing punctuation under the cursor, and it is delimited from surrounding text while preserving existing prefix whitespace. The cursor then moves to the beginning of the inserted word. Other transcripts are appended to the cursor's current line, preserving interior blank lines; the cursor moves to the beginning of the final resulting line. The first runtime directory resolved by the plugin remains fixed for the Neovim session; repeated `setup()` calls for the same directory are no-ops, while attempts to switch it fail. `setup(opts)`, `set_target(id)`, and `load(id)` return `true` on success or `nil, err` on failure. Failures emit one error notification inside the affected Neovim instance without raising another error, including failures received through remote delivery. Transcript-loading notifications include the ID, and a successful retry emits an informational notification with the retried ID. A successful actual target switch emits an informational notification; selecting an already-current target does not.

## Troubleshooting

1. If `setup()` reports that the runtime directory or daemon socket is unavailable, start the `talk2text` daemon first and confirm that the plugin and daemon resolve the same runtime directory.
2. If transcripts do not reach Neovim, confirm that `talk2text-nvim` is on `PATH`, `nvim` is on `PATH`, and the selected Neovim instance is still running.
3. If no selected target exists and default-editor startup fails, confirm that `TALK2TEXT_NVIM_CMD` can start Neovim directly, or configure `TALK2TEXT_NVIM_LAUNCH_CMD` when a launch command is needed.
4. If an existing default editor does not come forward, configure `TALK2TEXT_NVIM_FOCUS_CMD` for your window manager.
5. If `qq` cannot copy from the default editor, check Neovim's `+` clipboard provider with `:checkhealth`.
6. Run `talk2text-nvim <kind> <absolute-transcript-path>` directly from a shell to inspect errors written to stderr.

More detailed behavior is documented in [docs/spec.md](docs/spec.md).

## Contributing

Run the test suite from the repository root:

```sh
tests/run.sh
```

The tests start local headless Neovim servers and create temporary Unix sockets under `/tmp`.
