#!/usr/bin/env bash

set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_dir=$(mktemp -d /tmp/talk2text-nvim-tests.XXXXXX)
server_pid=
export NVIM_LOG_FILE=$test_dir/nvim.log

cleanup() {
  if [[ -n $server_pid ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf -- "$test_dir"
}
trap cleanup EXIT

fail() {
  printf 'test failure: %s\n' "$*" >&2
  return 1
}

wait_for_file() {
  local path=$1
  local process=$2
  local log=$3
  local attempt
  for (( attempt = 0; attempt < 100; attempt++ )); do
    if [[ -f $path ]]; then
      return 0
    fi
    if ! kill -0 "$process" 2>/dev/null; then
      sed -n '1,120p' "$log" >&2
      fail "Neovim exited before creating $path"
      return 1
    fi
    sleep 0.05
  done
  sed -n '1,120p' "$log" >&2
  fail "timed out waiting for $path"
}

TALK2TEXT_TEST_DIR="$test_dir/plugin" \
  NVIM_LOG_FILE="$test_dir/plugin-nvim.log" \
  nvim --headless -u NONE -i NONE -n \
  --cmd "set runtimepath^=$project_root" \
  -l "$project_root/tests/plugin.lua"

if "$project_root/bin/talk2text-nvim" blank /transcripts/absent.txt 2>/dev/null; then
  fail "filesystem root was accepted as the runtime directory"
fi

runtime_dir=$test_dir/runtime
transcript_dir=$runtime_dir/transcripts
socket=$test_dir/server.sock
server_log=$test_dir/server.log
notify_log=$test_dir/notify.log
focus_log=$test_dir/focus.log
mkdir -p "$transcript_dir"

export TALK2TEXT_TEST_NOTIFY_LOG=$notify_log
export TALK2TEXT_TEST_FOCUS_LOG=$focus_log
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_NOTIFY_CMD='printf "%s\n" "$1" >> "$TALK2TEXT_TEST_NOTIFY_LOG"'
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_FOCUS_CMD='printf "focused\n" >> "$TALK2TEXT_TEST_FOCUS_LOG"'

NVIM_LOG_FILE="$test_dir/server-nvim.log" \
  nvim --headless -u NONE -i NONE -n --listen "$socket" \
  --cmd "set runtimepath^=$project_root" \
  -c "lua require('talk2text').setup({runtime_dir='$runtime_dir'}); require('talk2text').set_target()" \
  >"$server_log" 2>&1 &
server_pid=$!
wait_for_file "$runtime_dir/nvim-target" "$server_pid" "$server_log"

space_path=$transcript_dir/with\ space.txt
printf 'first line\nsecond line\n' > "$space_path"
"$project_root/bin/talk2text-nvim" text "$space_path"
[[ ! -e $space_path ]] || fail "successful RPC load retained transcript"
loaded=$(nvim --server "$socket" --remote-expr 'join(getline(1,"$"), "|")')
[[ $loaded == 'first line|second line' ]] || fail "unexpected RPC buffer: $loaded"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_default_target()")' >/dev/null
printf '/tmp/stale-talk2text-nvim.sock\n' > "$runtime_dir/nvim-target"
nvim --server "$socket" --remote-expr 'execute("enew")' >/dev/null
fallback_path=$transcript_dir/fallback.txt
printf 'fallback' > "$fallback_path"
"$project_root/bin/talk2text-nvim" text "$fallback_path"
[[ ! -e $runtime_dir/nvim-target ]] || fail "stale explicit target was not removed"
[[ -f $runtime_dir/default-nvim-target ]] || fail "default target was removed during fallback"
fallback=$(nvim --server "$socket" --remote-expr 'getline(1)')
[[ $fallback == fallback ]] || fail "default target did not load into its current buffer"
[[ $(sed -n '$p' "$focus_log") == focused ]] || fail "default editor was not focused"

nvim --server "$socket" --remote-expr 'execute("setlocal nomodifiable")' >/dev/null
failed_path=$transcript_dir/failed.txt
printf 'retry me' > "$failed_path"
if "$project_root/bin/talk2text-nvim" text "$failed_path" >/dev/null 2>&1; then
  fail "reachable target load failure returned success"
fi
[[ -f $failed_path ]] || fail "failed target load removed transcript"
[[ -f $runtime_dir/default-nvim-target ]] || fail "reachable load failure removed target"
nvim --server "$socket" --remote-expr 'execute("setlocal modifiable")' >/dev/null
retry_result=$(nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\")._remote_load(nil)")')
[[ $retry_result == talk2text-ok ]] || fail "failed transcript retry did not succeed"
[[ ! -e $failed_path ]] || fail "successful retry retained transcript"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_target()")' >/dev/null
short_path=$transcript_dir/short.txt
: > "$short_path"
"$project_root/bin/talk2text-nvim" short "$short_path"
[[ ! -e $runtime_dir/nvim-target ]] || fail "short transcript did not reset explicit target"
[[ -f $runtime_dir/default-nvim-target ]] || fail "short transcript changed default target"
[[ ! -e $short_path ]] || fail "short transcript was not removed"

second_short_path=$transcript_dir/second-short.txt
: > "$second_short_path"
"$project_root/bin/talk2text-nvim" short "$second_short_path"
[[ ! -e $second_short_path ]] || fail "short transcript was not removed when the explicit target was absent"

failed_short_path=$transcript_dir/failed-short.txt
: > "$failed_short_path"
mkdir "$runtime_dir/nvim-target"
if "$project_root/bin/talk2text-nvim" short "$failed_short_path" 2>/dev/null; then
  fail "short succeeded when its explicit target could not be removed"
fi
[[ ! -e $failed_short_path ]] || fail "short retained its transcript after a later target-reset failure"
rmdir "$runtime_dir/nvim-target"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_target()")' >/dev/null
short_cleanup_dir=$transcript_dir/short-cleanup-dir
mkdir "$short_cleanup_dir"
"$project_root/bin/talk2text-nvim" short "$short_cleanup_dir" 2>/dev/null
[[ -d $short_cleanup_dir ]] || fail "short unexpectedly removed a transcript directory"
[[ ! -e $runtime_dir/nvim-target ]] || fail "short cleanup failure prevented target reset"
rmdir "$short_cleanup_dir"

blank_path=$transcript_dir/blank.txt
: > "$blank_path"
"$project_root/bin/talk2text-nvim" blank "$blank_path"
[[ ! -e $blank_path ]] || fail "blank transcript was not removed"

blank_cleanup_dir=$transcript_dir/blank-cleanup-dir
mkdir "$blank_cleanup_dir"
"$project_root/bin/talk2text-nvim" blank "$blank_cleanup_dir" 2>/dev/null
[[ -d $blank_cleanup_dir ]] || fail "blank unexpectedly removed a transcript directory"
rmdir "$blank_cleanup_dir"
[[ $(grep -Fxc 'Target reset to default Neovim' "$notify_log") -eq 3 ]] || fail "short notifications did not cover cleanup failure"
[[ $(grep -Fxc 'Blank transcript' "$notify_log") -eq 2 ]] || fail "blank notification did not survive cleanup failure"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_target()")' >/dev/null
printf 'replacement\n' > "$runtime_dir/nvim-target"
nvim --server "$socket" --remote-expr 'execute("qa!")' >/dev/null 2>&1 || true
wait "$server_pid" || true
server_pid=
[[ $(sed -n '1p' "$runtime_dir/nvim-target") == replacement ]] || fail "quit cleanup removed a replacement target"
[[ ! -e $runtime_dir/default-nvim-target ]] || fail "quit cleanup retained its default target"

startup_base=$test_dir/xdg
startup_runtime=$startup_base/talk2text
mkdir -p "$startup_runtime/transcripts"
startup_path=$startup_runtime/transcripts/default\ editor.txt
startup_cwd_log=$test_dir/startup-cwd.log
printf 'started by terminal hook' > "$startup_path"
export TALK2TEXT_TEST_ROOT=$project_root
export TALK2TEXT_TEST_CWD_LOG=$startup_cwd_log
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_TERMINAL_CMD='pwd > "$TALK2TEXT_TEST_CWD_LOG"; nvim --headless -u NONE -i NONE -n --cmd "set runtimepath^=$TALK2TEXT_TEST_ROOT" "$@" +qa!'
(
  cd "$test_dir"
  XDG_RUNTIME_DIR=$startup_base NVIM_LOG_FILE="$test_dir/startup-nvim.log" \
    "$project_root/bin/talk2text-nvim" text "$startup_path"
)
[[ $(cat "$startup_cwd_log") == "$test_dir" ]] || fail "terminal hook did not inherit the output command working directory"
[[ ! -e $startup_path ]] || fail "default editor startup retained transcript"
[[ ! -e $startup_runtime/default-nvim-target ]] || fail "default editor quit retained target"

printf 'all tests passed\n'
