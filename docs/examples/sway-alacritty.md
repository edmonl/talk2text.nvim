# Sway and Alacritty

This is a starting point for a personal copy of `talk2text-nvim` or for a shell wrapper, not a project default.

Set hooks in the environment:

```sh
export TALK2TEXT_NVIM_TERMINAL_CMD='exec alacritty --class talk2text-editor --title talk2text --command nvim "$@"'
export TALK2TEXT_NVIM_FOCUS_CMD="swaymsg '[app_id=\"talk2text-editor\"] focus' >/dev/null"
```

For a personal copy, set the terminal hook to:

```sh
exec alacritty --class talk2text-editor --title talk2text --command nvim "$@"
```

For a personal copy, set the focus hook to:

```sh
swaymsg '[app_id="talk2text-editor"] focus' >/dev/null
```

The terminal hook receives the generated Neovim startup arguments as `"$@"`. The focus hook receives no arguments.
