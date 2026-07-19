# Command Spec

The executable command is named:

```text
talk2text-nvim
```

The initial command implementation is a Bash script. The public contract is the command-line behavior in this spec, so the implementation may later be replaced without changing how users invoke it.

It is used as the `talk2text` output command, so it accepts the same argument shape:

```sh
talk2text-nvim <kind> <path>
```

Supported kinds:

1. `text`
2. `blank`
3. `short`

Unknown arguments or invalid invocation exit with a nonzero status. No detailed logging is required because output command logs may be discarded by `talk2text`.

Exit status has only two semantic categories:

1. `0`: success.
2. Any nonzero status: failure.

Specific nonzero status values are implementation details.

Notifications are best-effort. A missing or failing notification command does not change an otherwise successful exit status.

For `blank` and `short`, an already-absent transcript file counts as already cleaned. Transcript cleanup is best-effort: failure is reported but does not interrupt the remaining handling or change an otherwise successful exit status.

# Command Hooks

The command uses shell-configurable hooks for notifications, Neovim control, default editor startup, and editor focus. Defaults:

1. `TALK2TEXT_NVIM_NOTIFY_CMD`: `notify-send -t 5000 Talk2text "$@"`
2. `TALK2TEXT_NVIM_CLIENT_CMD`: `nvim "$@"`
3. `TALK2TEXT_NVIM_TERMINAL_CMD`: empty.
4. `TALK2TEXT_NVIM_FOCUS_CMD`: empty.

The client and terminal hooks are required; notification and focus hooks are optional. A text delivery requires a non-empty client hook. When no usable target exists, it also requires a non-empty terminal hook to start the default editor. A missing required hook causes failure when it is needed and does not trigger target fallback. Optional hooks are skipped when empty.

Each setting is read from its correspondingly named environment variable; when it is unset, the listed default applies. Each non-empty setting is trusted shell code run with `sh -c`. Hooks inherit the output command's current working directory. The notification, client, and terminal hooks receive generated arguments as shell positional parameters, represented by `"$@"` in the defaults. Runtime values are supplied this way and must never be interpolated into hook code. The notification hook receives the notification body as one argument. The client hook receives Neovim client arguments for server probes and loads. The terminal hook receives Neovim startup arguments. The focus hook receives no generated arguments.

Users may set hooks as environment variables for an invocation or wrapper, or copy the command and adapt its defaults. The distributed client default satisfies the client-hook requirement, but users must configure the empty terminal hook before default-editor startup can work. See the [Sway and Alacritty example](../examples/sway-alacritty.md) for one starting point.

# `text <path>`

For normal text transcripts:

1. Infer the runtime directory from `path`.
2. If `<runtime_dir>/nvim-target` exists and contains a usable Neovim server socket, call `require("talk2text").load(path)` in that server through the Neovim socket and read the response.
3. If loading into `nvim-target` succeeds, exit `0`.
4. If `nvim-target` is missing, empty, stale, or unusable, try `<runtime_dir>/default-nvim-target`.
5. If `<runtime_dir>/default-nvim-target` exists and contains a usable Neovim server socket, call `require("talk2text").load(path)` in that server through the Neovim socket and read the response.
6. If loading into `default-nvim-target` succeeds, focus the default editor window when applicable, then exit `0`.
7. If neither target file can be used, invoke the terminal hook with startup arguments for a new default Neovim editor.
8. Apply the main specification's stale-target cleanup rule before continuing to the next fallback.

Successful loads remove the transcript file. If a target is reachable but the load fails, returns `nil, err`, or raises a Lua error, the command exits nonzero instead of falling back. This includes failure to remove the file after it was loaded; it does not retry the load, because retrying could append the same transcript twice. The file remains for `talk2text`'s next startup cleanup.

If a target cannot be reached as a Neovim server, the command treats that target as stale or unusable and falls back according to the target resolution order.

# `blank <path>`

For blank transcripts:

1. Do not load text into Neovim.
2. Do not start or focus the default editor.
3. Do not change `nvim-target` or `default-nvim-target`.
4. Attempt to remove `path`.
5. Emit a notification with the configured notification command.
6. Exit `0`.

Default notification:

```text
title: Talk2text
body: Blank transcript
```

# `short <path>`

For short transcripts, the command is used as a shortcut to switch future text output back to the default Neovim editor:

1. Infer the runtime directory from `path`.
2. Attempt to remove `path`.
3. Delete `<runtime_dir>/nvim-target` if it exists.
4. Do not delete or change `<runtime_dir>/default-nvim-target`.
5. Do not start or focus the default editor.
6. Emit a notification with the configured notification command, whether or not `nvim-target` existed.
7. Exit `0`.

Default notification:

```text
title: Talk2text
body: Target reset to default Neovim
```

# Default Neovim Editor

The default editor is the command-started Neovim instance used when no explicit target is usable.

The command uses `<runtime_dir>/default-nvim-target` to detect and reuse an existing default editor. If that target file contains a usable Neovim server socket, the command loads the transcript through that server instead of starting a new editor.

When an existing default editor is reused, the command should focus its window when that window exists. Focusing is best-effort; a missing or failing focus command is not required for a successful transcript load.

# Default Editor Startup

Default editor startup is the part of the spec that chooses how to make a Neovim UI appear. It runs the configured terminal hook without changing the output command's current working directory. The distributed command leaves that hook empty, so users must set it as an environment variable or copy and adapt the command before default-editor startup can work. The [Sway and Alacritty example](../examples/sway-alacritty.md) is illustrative rather than a project default.

The command invokes the terminal hook but does not poll for target registration or make the initial load through the client hook. The terminal hook may remain running for the editor session or detach; its process lifetime and exit status are not proof that the initial transcript load succeeded.

Missing required terminal or client hooks cause failure when they are needed.

The default editor uses the user's normal Neovim configuration.

The command passes default-editor startup behavior and the transcript path to Neovim through startup commands or command-line arguments. It does not call `setup()`; plugin configuration comes from the user's normal Neovim configuration. No marker environment variable is required.

The launched Neovim process starts in a no-file buffer. It should not open the transcript file as the editable buffer. The plugin handles default-editor startup by registering the new Neovim server as `default-nvim-target`, loading the transcript, and configuring the default editor buffer mapping described in the plugin spec. It removes the transcript only after a successful load and reports an initial-load failure in Neovim. A failed initial load remains available for retry and for `talk2text`'s next startup cleanup. See [ADR 0004](../decisions/0004-delegate-default-editor-transcript-cleanup-to-neovim.md).
