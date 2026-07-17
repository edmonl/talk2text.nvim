# Smoke Test

Goal: confirm that a transcript can be loaded into a Neovim target and removed after successful delivery.

Steps:

1. Start the `talk2text` daemon so it creates the runtime directory.
2. Open Neovim with this plugin available.
3. Run `:lua require("talk2text").set_target()`.
4. Create a transcript file under `<runtime_dir>/transcripts/`.
5. Run `talk2text-nvim text <path-to-transcript>`.
6. Confirm the transcript text was appended to the Neovim buffer.
7. Confirm the transcript file no longer exists.

State affected:

1. `<runtime_dir>/nvim-target` may be written by the test.
2. The current Neovim buffer receives appended text.
3. The transcript file is removed.

Cleanup:

1. Quit the Neovim instance used as the target.
2. Confirm `<runtime_dir>/nvim-target` is removed, or remove it manually if the run was interrupted.
