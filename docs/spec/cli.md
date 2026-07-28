# Command Spec

The executable command is named:

```text
talk2text-nvim
```

This document defines the command's externally observable behavior. Its implementation language and internal structure are not part of the public contract.

It is used as the `talk2text` output command, so it accepts the same argument shape:

```sh
TALK2TEXT_OUTPUT_KIND=<kind> \
TALK2TEXT_NOTIFY_CMD=<notification-command> \
talk2text-nvim <path>
```

The transcript path is the command's only argument. `TALK2TEXT_OUTPUT_KIND` and `TALK2TEXT_NOTIFY_CMD` are daemon-owned command metadata. `TALK2TEXT_NOTIFY_CMD` is the user-configured notification executable or an empty value when notifications are disabled. Supported output-kind values are:

1. `text`
2. `blank`
3. `short`

Unknown metadata values, extra arguments, or other invalid invocations exit with a nonzero status.

For every kind, the transcript filename must have the canonical form `<positive-id>`, where the ID is a base-10 positive integer without a sign or leading zero.

Exit status is `0` for success and nonzero for failure. Specific nonzero values are not part of the contract.

Notifications are best-effort. A missing or failing notification command does not change an otherwise successful exit status. Informational Neovim integration events use `TALK2TEXT_NOTIFY_LEVEL=info` and `TALK2TEXT_NOTIFY_CODE=nvim`. Output failures use `TALK2TEXT_NOTIFY_LEVEL=error` and `TALK2TEXT_NOTIFY_CODE=output-command`. Notification text is user-facing and should be chosen for the intended UX; it may include or omit the transcript ID. Diagnostic messages written to standard error should include the transcript ID whenever it is available.

For `blank` and `short`, an already-absent transcript file counts as already cleaned. Transcript cleanup is best-effort: failure is reported but does not interrupt the remaining handling or change an otherwise successful exit status.

# Command Hooks

The command uses shell-configurable hooks for default editor startup and editor focus. Defaults:

1. `TALK2TEXT_NVIM_LAUNCH_CMD`: `nvim`.
2. `TALK2TEXT_NVIM_FOCUS_CMD`: empty.

The launch command is required only when a new default editor must be started. Existing-target delivery does not require it. The focus hook is optional and skipped when empty. A missing required setting causes failure when it is needed.

Hook values have surrounding whitespace removed. Each non-empty value is trusted shell code run with `sh -c`. Hooks inherit the output command's environment and working directory except for variables beginning with `TALK2TEXT_NVIM_`. The launch command receives the transcript path as its only generated argument; the focus command receives none.

The notification command is not a shell hook. The command uses `TALK2TEXT_NOTIFY_CMD` as an executable and appends the notification message as its only argument. Notifications use the same environment inheritance and `TALK2TEXT_NVIM_` filtering as hooks, with `TALK2TEXT_NOTIFY_LEVEL` and `TALK2TEXT_NOTIFY_CODE` set for each invocation.

Notifications and focus hooks run asynchronously and are best-effort. Startup errors and subprocess stderr are written to the output command's stderr. Default-editor startup remains attached and propagates the launch command's exit status.

# `TALK2TEXT_OUTPUT_KIND=text`

For normal text transcripts:

1. Infer the runtime directory from `path`.
2. Try `nvim-target`, then `default-nvim-target`, following the target rules in the [main spec](../spec.md).
3. When the default target handles the transcript, run the best-effort focus hook.
4. When neither target is usable, start a default editor through the launch command.

Successful delivery to an existing target removes the transcript. A reachable target failure is fatal and is not retried elsewhere, preventing duplicate insertion. A newly launched editor owns subsequent handling of the transcript path.

# `TALK2TEXT_OUTPUT_KIND=blank`

Blank transcripts are removed best-effort and emit an `info` / `nvim` notification. They do not load text, change targets, or start or focus an editor.

# `TALK2TEXT_OUTPUT_KIND=short`

Short transcripts switch future output back to the default editor. The command removes the transcript best-effort, deletes `nvim-target`, retains `default-nvim-target`, and emits an `info` / `nvim` notification. It does not start or focus an editor. An already-absent target is successful; a target-reset failure is fatal.

# Default Editor Startup

The launch command is complete, defaults to `nvim`, and receives the transcript path as its only generated argument. It runs in the output command's working directory. A missing launch command is fatal when startup is needed.

If default-editor startup launches a graphical application, the output command must have the graphical-session environment required by that application. This is especially relevant when a long-running service starts before the graphical session and later invokes the output command.

With the default launch command, Neovim uses the user's normal configuration and opens the transcript as a normal file. Custom launch integrations own any additional target registration, buffer setup, or cleanup.
