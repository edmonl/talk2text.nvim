# Command Spec

The executable command is named:

```text
talk2text-nvim
```

This document defines the command's externally observable behavior. Its implementation language and internal structure are not part of the public contract.

It is used as the `talk2text` output command, so it accepts the same argument shape:

```sh
talk2text-nvim <kind> <path>
```

Supported kinds:

1. `text`
2. `blank`
3. `short`

Unknown arguments or invalid invocation exit with a nonzero status. No detailed logging is required because output command logs may be discarded by `talk2text`.

For every kind, the transcript filename must have the canonical form `<positive-id>`, where the ID is a base-10 positive integer without a sign or leading zero. The command derives that ID after validating the path and passes only the ID to Neovim. Runtime selection inside the plugin remains governed by plugin configuration; internal remote-load and default-start adapters do not receive a runtime directory.

Exit status has only two semantic categories:

1. `0`: success.
2. Any nonzero status: failure.

Specific nonzero status values are implementation details.

Notifications are best-effort. A missing or failing notification command does not change an otherwise successful exit status.

For `blank` and `short`, an already-absent transcript file counts as already cleaned. Transcript cleanup is best-effort: failure is reported but does not interrupt the remaining handling or change an otherwise successful exit status.

# Command Hooks

The command uses shell-configurable hooks for notifications, default editor startup, and editor focus. Defaults:

1. `TALK2TEXT_NVIM_NOTIFY_CMD`: `notify-send -a talk2text -u normal -t 5000 Talk2text` when `notify-send` is available on `PATH`; otherwise empty.
2. `TALK2TEXT_NVIM_LAUNCH_CMD`: `nvim`.
3. `TALK2TEXT_NVIM_FOCUS_CMD`: empty.

The launch command is required only when a new default editor must be started. Existing-target delivery does not require it. The notification and focus hooks are optional and skipped when empty. A missing required setting causes failure when it is needed.

Each setting is read from its correspondingly named environment variable and has surrounding whitespace removed. When the notification variable is unset, the command checks for `notify-send` while constructing its configuration and enables the listed default only when the executable is available. Explicit notification values, including an empty value or the default text, are used without an availability check. The other unset settings use their listed defaults. Each non-empty setting is trusted shell code run with `sh -c`. Hooks inherit the output command's current environment and working directory. Generated arguments are appended internally, so settings do not include `"$@"`. Runtime values are supplied as shell positional parameters and must never be interpolated into hook code. The notification command receives the notification body as one argument. It reports blank and short transcripts, successful stale-target deletion, and fatal target errors; fatal target messages begin with `Error: `. Existing-target probes and loads use MessagePack-RPC directly without invoking the launch command. For default-editor startup, generated Neovim startup arguments are appended to the complete launch command. The focus command receives no generated arguments.

Notification and focus hooks run asynchronously after their shell process starts successfully. Immediate shell-start failures and hook stderr are written to the output command's stderr without changing its exit status. Default-editor startup remains attached to the caller and propagates the configured shell command's exit status. Notification and focus hooks retain their best-effort result semantics.

Users may set command hooks as environment variables for an invocation or wrapper, or copy the command and adapt its defaults. The distributed `nvim` launch default satisfies the launch-command requirement.

# `text <path>`

For normal text transcripts:

1. Infer the runtime directory from `path`.
2. Derive the positive transcript ID from the canonical filename.
3. If `<runtime_dir>/nvim-target` exists and contains a usable Neovim server socket, call the plugin's internal load adapter with the ID in that server through the Neovim socket and read the response.
4. If loading into `nvim-target` succeeds, exit `0`.
5. If `nvim-target` is missing, zero-byte, or stale, try `<runtime_dir>/default-nvim-target`. A malformed or non-absolute target is fatal instead.
6. If `<runtime_dir>/default-nvim-target` exists and contains a usable Neovim server socket, call the same internal load adapter with the ID and read the response.
7. If loading into `default-nvim-target` succeeds, focus the default editor window when applicable, then exit `0`.
8. If neither target file is present or reachable, start a new default Neovim editor through the configured launch command.
9. Apply the main specification's stale-target cleanup rule before continuing to the next fallback.

Successful loads remove the transcript file. If a target is reachable but the load returns `false, err` or raises a Lua error, the command exits nonzero instead of falling back. This includes failure to remove the file after it was loaded; it does not retry the load, because retrying could append the same transcript twice. The file remains for `talk2text`'s next startup cleanup.

If an absolute target cannot be reached as a Neovim server, the command treats that target as stale and falls back according to the target resolution order after conditionally deleting it. Successful stale deletion emits a stale-target notification. Target read errors, a nonempty blank first line, a non-absolute socket path, cleanup failures, and reachable-target load failures are fatal and emit notifications beginning with `Error: `.

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
7. Exit `0` after a successful target reset, including when `nvim-target` is already absent. Exit nonzero if the target cannot be reset.

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

Default editor startup is the part of the spec that chooses how to make a Neovim UI appear. The launch command is a complete command, defaulting to `nvim`, and receives the generated startup arguments. Launching does not change the output command's current working directory.

If default-editor startup launches a graphical application, the output command must have the graphical-session environment required by that application. This is especially relevant when a long-running service starts before the graphical session and later invokes the output command.

The command starts the new Neovim instance but does not poll for target registration or make the initial load through a separate client call. The resulting process may remain running for the editor session or detach; its process lifetime and exit status are not proof that the initial transcript load succeeded.

A missing launch command causes failure when default-editor startup is needed.

The default editor uses the user's normal Neovim configuration.

The command passes default-editor startup behavior and the transcript ID to Neovim through startup commands or command-line arguments. It does not call `setup()`; plugin configuration comes from the user's normal Neovim configuration. No marker environment variable is required.

The launched Neovim process starts in a no-file buffer. It should not open the transcript file as the editable buffer. The plugin handles default-editor startup by registering the new Neovim server as `default-nvim-target`, loading the transcript, and configuring the default editor buffer mapping described in the plugin spec. It removes the transcript only after a successful load and reports an initial-load failure in Neovim. A failed initial load remains available for retry and for `talk2text`'s next startup cleanup. See [ADR 0004](../decisions/0004-delegate-default-editor-transcript-cleanup-to-neovim.md).
