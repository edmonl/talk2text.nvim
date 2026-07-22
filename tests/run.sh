#!/usr/bin/env bash

set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if [[ -z ${TALK2TEXT_NVIM_TEST_COMMAND+x} ]]; then
  build_dir=$(mktemp -d /tmp/talk2text-nvim-build.XXXXXX)
  # shellcheck disable=SC2329 # Invoked indirectly by the EXIT trap.
  cleanup_build() {
    rm -rf -- "$build_dir"
  }
  trap cleanup_build EXIT

  go build -o "$build_dir/talk2text-nvim-go" "$project_root"
  TALK2TEXT_NVIM_TEST_COMMAND="$project_root/bin/talk2text-nvim" "$0"
  TALK2TEXT_NVIM_TEST_COMMAND="$build_dir/talk2text-nvim-go" TALK2TEXT_NVIM_TEST_DIRECT_RPC=1 "$0"
  printf 'all implementations passed\n'
  exit 0
fi

command=$TALK2TEXT_NVIM_TEST_COMMAND
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

wait_for_absence() {
  local path=$1
  local attempt
  for (( attempt = 0; attempt < 100; attempt++ )); do
    if [[ ! -e $path ]]; then
      return 0
    fi
    sleep 0.05
  done
  fail "timed out waiting for $path to disappear"
}

wait_for_line_count() {
  local path=$1
  local line=$2
  local expected=$3
  local attempt count
  for (( attempt = 0; attempt < 100; attempt++ )); do
    count=$(grep -Fxc "$line" "$path" 2>/dev/null || true)
    if [[ $count -eq $expected ]]; then
      return 0
    fi
    sleep 0.05
  done
  fail "timed out waiting for $expected instances of '$line' in $path"
}

TALK2TEXT_TEST_DIR="$test_dir/plugin" \
  NVIM_LOG_FILE="$test_dir/plugin-nvim.log" \
  nvim --headless -u NONE -i NONE -n \
  --cmd "set runtimepath^=$project_root" \
  -l "$project_root/tests/plugin.lua"

if "$command" blank /transcripts/absent.txt 2>/dev/null; then
  fail "filesystem root was accepted as the runtime directory"
fi

runtime_dir=$test_dir/runtime
transcript_dir=$runtime_dir/transcripts
socket=$test_dir/server.sock
server_log=$test_dir/server.log
notify_log=$test_dir/notify.log
focus_log=$test_dir/focus.log
mkdir -p "$transcript_dir"

malformed_path=$transcript_dir/01.txt
: > "$malformed_path"
if "$command" blank "$malformed_path" 2>/dev/null; then
  fail "malformed transcript ID was accepted"
fi
[[ -e $malformed_path ]] || fail "rejected malformed transcript was removed"

export TALK2TEXT_TEST_NOTIFY_LOG=$notify_log
export TALK2TEXT_TEST_FOCUS_LOG=$focus_log
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_NOTIFY_CMD='printf "%s\n" >> "$TALK2TEXT_TEST_NOTIFY_LOG"'
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_FOCUS_CMD='printf "focused\n" >> "$TALK2TEXT_TEST_FOCUS_LOG"'

NVIM_LOG_FILE="$test_dir/server-nvim.log" \
  nvim --headless -u NONE -i NONE -n --listen "$socket" \
  --cmd "set runtimepath^=$project_root" \
  -c "lua vim.fn.serverstart('$runtime_dir/daemon.sock'); require('talk2text').setup({runtime_dir='$runtime_dir'}); require('talk2text').set_target()" \
  >"$server_log" 2>&1 &
server_pid=$!
wait_for_file "$runtime_dir/nvim-target" "$server_pid" "$server_log"

first_path=$transcript_dir/1.txt
printf 'first line\nsecond line\n' > "$first_path"
if [[ ${TALK2TEXT_NVIM_TEST_DIRECT_RPC-0} -eq 1 ]]; then
  export TALK2TEXT_NVIM_CMD='exit 97'
fi
"$command" text "$first_path"
unset TALK2TEXT_NVIM_CMD
[[ ! -e $first_path ]] || fail "successful RPC load retained transcript"
loaded=$(nvim --server "$socket" --remote-expr 'join(getline(1,"$"), "|")')
[[ $loaded == 'first line|second line' ]] || fail "unexpected RPC buffer: $loaded"

default_setup_path=$transcript_dir/2.txt
: > "$default_setup_path"
nvim --server "$socket" --remote-expr 'execute("enew")' >/dev/null
nvim --server "$socket" --remote-expr "luaeval('require(\"talk2text\")._default_start(2)')" >/dev/null
[[ ! -e $default_setup_path ]] || fail "default startup retained its setup transcript"
printf '/tmp/stale-talk2text-nvim.sock\n' > "$runtime_dir/nvim-target"
nvim --server "$socket" --remote-expr 'execute("enew")' >/dev/null
fallback_path=$transcript_dir/3.txt
printf 'fallback' > "$fallback_path"
"$command" text "$fallback_path"
[[ ! -e $runtime_dir/nvim-target ]] || fail "stale explicit target was not removed"
[[ -f $runtime_dir/default-nvim-target ]] || fail "default target was removed during fallback"
fallback=$(nvim --server "$socket" --remote-expr 'getline(1)')
[[ $fallback == fallback ]] || fail "default target did not load into its current buffer"
wait_for_line_count "$focus_log" focused 1
[[ $(sed -n '$p' "$focus_log") == focused ]] || fail "default editor was not focused"

nvim --server "$socket" --remote-expr 'execute("setlocal nomodifiable")' >/dev/null
failed_path=$transcript_dir/4.txt
printf 'retry me' > "$failed_path"
if "$command" text "$failed_path" >/dev/null 2>&1; then
  fail "reachable target load failure returned success"
fi
[[ -f $failed_path ]] || fail "failed target load removed transcript"
[[ -f $runtime_dir/default-nvim-target ]] || fail "reachable load failure removed target"
nvim --server "$socket" --remote-expr 'execute("setlocal modifiable")' >/dev/null
retry_result=$(nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\")._remote_load(0)")')
[[ $retry_result == ok ]] || fail "failed transcript retry did not succeed"
[[ ! -e $failed_path ]] || fail "successful retry retained transcript"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_target()")' >/dev/null
short_path=$transcript_dir/5.txt
: > "$short_path"
"$command" short "$short_path"
[[ ! -e $runtime_dir/nvim-target ]] || fail "short transcript did not reset explicit target"
[[ -f $runtime_dir/default-nvim-target ]] || fail "short transcript changed default target"
[[ ! -e $short_path ]] || fail "short transcript was not removed"

second_short_path=$transcript_dir/6.txt
: > "$second_short_path"
"$command" short "$second_short_path"
[[ ! -e $second_short_path ]] || fail "short transcript was not removed when the explicit target was absent"

failed_short_path=$transcript_dir/7.txt
: > "$failed_short_path"
mkdir "$runtime_dir/nvim-target"
if "$command" short "$failed_short_path" 2>/dev/null; then
  fail "short succeeded when its explicit target could not be removed"
fi
[[ ! -e $failed_short_path ]] || fail "short retained its transcript after a later target-reset failure"
rmdir "$runtime_dir/nvim-target"

nvim --server "$socket" --remote-expr 'luaeval("require(\"talk2text\").set_target()")' >/dev/null
short_cleanup_dir=$transcript_dir/8.txt
mkdir "$short_cleanup_dir"
"$command" short "$short_cleanup_dir" 2>/dev/null
[[ -d $short_cleanup_dir ]] || fail "short unexpectedly removed a transcript directory"
[[ ! -e $runtime_dir/nvim-target ]] || fail "short cleanup failure prevented target reset"
rmdir "$short_cleanup_dir"

blank_path=$transcript_dir/9.txt
: > "$blank_path"
"$command" blank "$blank_path"
[[ ! -e $blank_path ]] || fail "blank transcript was not removed"

blank_cleanup_dir=$transcript_dir/10.txt
mkdir "$blank_cleanup_dir"
"$command" blank "$blank_cleanup_dir" 2>/dev/null
[[ -d $blank_cleanup_dir ]] || fail "blank unexpectedly removed a transcript directory"
rmdir "$blank_cleanup_dir"
wait_for_line_count "$notify_log" 'Target reset to default' 3
wait_for_line_count "$notify_log" 'Blank transcript' 2

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
startup_path=$startup_runtime/transcripts/1.txt
startup_cwd_log=$test_dir/startup-cwd.log
printf 'started by launch command' > "$startup_path"
export TALK2TEXT_TEST_ROOT=$project_root
export TALK2TEXT_TEST_CWD_LOG=$startup_cwd_log
export TALK2TEXT_TEST_STARTUP_RUNTIME=$startup_runtime
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_CMD='nvim --headless -u NONE -i NONE -n --cmd "set runtimepath^=$TALK2TEXT_TEST_ROOT" --cmd "lua vim.fn.serverstart(vim.env.TALK2TEXT_TEST_STARTUP_RUNTIME .. \"/daemon.sock\"); require(\"talk2text\").setup({runtime_dir=vim.env.TALK2TEXT_TEST_STARTUP_RUNTIME})"'
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_LAUNCH_CMD='pwd > "$TALK2TEXT_TEST_CWD_LOG"; run_launch_command() { "$@" +q; }; run_launch_command'
(
  cd "$test_dir"
  XDG_RUNTIME_DIR=$startup_base NVIM_LOG_FILE="$test_dir/startup-nvim.log" \
    "$command" text "$startup_path"
)
wait_for_absence "$startup_path"
[[ $(cat "$startup_cwd_log") == "$test_dir" ]] || fail "launch command did not inherit the output command working directory"
[[ ! -e $startup_path ]] || fail "default editor startup retained transcript"
wait_for_absence "$startup_runtime/default-nvim-target"
[[ ! -e $startup_runtime/default-nvim-target ]] || fail "default editor quit retained target"

direct_runtime=$test_dir/direct-runtime
direct_failure_path=$direct_runtime/transcripts/2.txt
direct_path=$direct_runtime/transcripts/3.txt
direct_args_log=$test_dir/direct-args.log
mkdir -p "$direct_runtime/transcripts"
printf 'retain after failed direct startup' > "$direct_failure_path"
export TALK2TEXT_NVIM_CMD='failed_direct_launch() { return 23; }; failed_direct_launch'
unset TALK2TEXT_NVIM_LAUNCH_CMD
if "$command" text "$direct_failure_path" 2>/dev/null; then
  fail "failed direct Neovim launch returned success"
fi
[[ -f $direct_failure_path ]] || fail "failed direct Neovim launch removed transcript"

printf 'started directly' > "$direct_path"
export TALK2TEXT_TEST_DIRECT_PATH=$direct_path
export TALK2TEXT_TEST_DIRECT_ARGS_LOG=$direct_args_log
# shellcheck disable=SC2016 # Expanded by the configured hook shell.
export TALK2TEXT_NVIM_CMD='direct_launch() { printf "%s\n" "$@" > "$TALK2TEXT_TEST_DIRECT_ARGS_LOG"; rm -f -- "$TALK2TEXT_TEST_DIRECT_PATH"; }; direct_launch'
unset TALK2TEXT_NVIM_LAUNCH_CMD
"$command" text "$direct_path"
wait_for_absence "$direct_path"
[[ $(sed -n '1p' "$direct_args_log") == -c ]] || fail "direct Neovim launch did not receive -c"
[[ $(sed -n '2p' "$direct_args_log") == 'lua require("talk2text")._default_start(3)' ]] || fail "direct Neovim launch received an unexpected startup command"

printf 'all tests passed for %s\n' "$command"
