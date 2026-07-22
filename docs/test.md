# Smoke Test

The automated suite runs the Go unit tests, builds the output command in a temporary directory, runs the Lua plugin tests in headless Neovim, and exercises the command against a real Neovim RPC server:

```sh
go test ./...
```

Goal: confirm that a transcript can be loaded into a Neovim target and removed after successful delivery.

Steps:

1. Start the `talk2text` daemon so it creates the runtime directory.
2. Open Neovim with this plugin available and configure `setup({ runtime_dir = "<runtime_dir>" })` explicitly.
3. Run `:lua require("talk2text").set_target()`.
4. Create a transcript file such as `<runtime_dir>/transcripts/1.txt`.
5. Run `talk2text-nvim text <path-to-transcript>`.
6. Confirm the transcript text was appended to the cursor's current line.
7. Confirm the transcript file no longer exists.

State affected:

1. `<runtime_dir>/nvim-target` may be written by the test.
2. The current Neovim buffer receives inserted text.
3. The transcript file is removed.

Cleanup:

1. Quit the Neovim instance used as the target.
2. Confirm `<runtime_dir>/nvim-target` is removed, or remove it manually if the run was interrupted.
