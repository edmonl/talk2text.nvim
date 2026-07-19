# Sway and Alacritty

This is a starting point for a personal copy of `talk2text-nvim` or for a shell wrapper, not a project default.

Set hooks in the environment:

```sh
export TALK2TEXT_NVIM_TERMINAL_CMD='exec alacritty --class talk2text-editor --title talk2text --command'
export TALK2TEXT_NVIM_FOCUS_CMD="swaymsg '[app_id=\"talk2text-editor\"] focus' >/dev/null"
```

For a personal copy, set the terminal command prefix to:

```sh
exec alacritty --class talk2text-editor --title talk2text --command
```

For a personal copy, set the focus hook to:

```sh
swaymsg '[app_id="talk2text-editor"] focus' >/dev/null
```

The configured Neovim command and generated startup arguments are appended to the terminal command. The focus command receives no arguments.
