local uv = vim.uv
local root = assert(vim.env.TALK2TEXT_TEST_DIR, "TALK2TEXT_TEST_DIR is required")
local runtime_dir = root .. "/runtime"
local transcript_dir = runtime_dir .. "/transcripts"

local function run()
  vim.fn.mkdir(transcript_dir, "p", 448)

  local daemon_socket = runtime_dir .. "/daemon.sock"
  assert(vim.fn.serverstart(daemon_socket) == daemon_socket, "failed to start fake daemon socket")
  local target_server = vim.v.servername

  local function fail(message)
    error(message, 0)
  end

  local function assert_equal(actual, expected, label)
    if not vim.deep_equal(actual, expected) then
      fail(("%s: expected %s, got %s"):format(label, vim.inspect(expected), vim.inspect(actual)))
    end
  end

  local function assert_true(value, label)
    if not value then
      fail(label)
    end
  end

  local function write_file(path, contents)
    local file = assert(io.open(path, "wb"))
    assert(file:write(contents))
    assert(file:close())
  end

  local function exists(path)
    return uv.fs_lstat(path) ~= nil
  end

  local function capture_notifications(callback)
    local notifications = {}
    local original_notify = vim.notify
    vim.notify = function(message, level)
      notifications[#notifications + 1] = { message, level }
    end
    local called, result, detail = xpcall(callback, debug.traceback)
    vim.notify = original_notify
    if not called then
      fail(result)
    end
    return notifications, result, detail
  end

  local runtime = require("talk2text.runtime")
  local original_xdg_runtime_dir = vim.env.XDG_RUNTIME_DIR
  local original_tmpdir = vim.env.TMPDIR
  local fallback_base = root .. "/fallback-base"
  local fallback_runtime = ("%s/run-%d/talk2text"):format(fallback_base, uv.getuid())
  vim.fn.mkdir(fallback_runtime, "p", 448)
  vim.env.XDG_RUNTIME_DIR = root .. "/missing-xdg-base"
  vim.env.TMPDIR = fallback_base

  local resolved, resolve_err = runtime.resolve()
  assert_equal(resolve_err, nil, "fallback runtime error")
  assert_equal(resolved, fallback_runtime, "missing XDG runtime fallback")

  local explicit_missing, explicit_err = runtime.resolve(root .. "/missing-explicit-runtime")
  assert_equal(explicit_missing, nil, "missing explicit runtime unexpectedly resolved")
  assert_true(explicit_err:match("unavailable") ~= nil, "missing explicit runtime omitted the cause")

  local invalid_xdg_base = root .. "/invalid-xdg-base"
  vim.fn.mkdir(invalid_xdg_base, "p", 448)
  write_file(invalid_xdg_base .. "/talk2text", "not a directory")
  vim.env.XDG_RUNTIME_DIR = invalid_xdg_base
  local invalid_runtime, invalid_err = runtime.resolve()
  assert_equal(invalid_runtime, nil, "invalid XDG runtime unexpectedly fell back")
  assert_true(invalid_err:match("not a directory") ~= nil, "invalid XDG runtime omitted the cause")

  local fallback_transcript_dir = fallback_runtime .. "/transcripts"
  vim.fn.mkdir(fallback_transcript_dir, "p", 448)
  vim.env.XDG_RUNTIME_DIR = root .. "/missing-xdg-base"
  local first_lazy_transcript = fallback_transcript_dir .. "/38.txt"
  write_file(first_lazy_transcript, "first lazy load.")
  local talk2text = require("talk2text")
  assert_true(talk2text.load(38), "initial lazy runtime resolution failed")

  vim.env.XDG_RUNTIME_DIR = invalid_xdg_base
  local second_lazy_transcript = fallback_transcript_dir .. "/39.txt"
  write_file(second_lazy_transcript, "second lazy load.")
  assert_true(talk2text.load(39), "cached lazy runtime was resolved again")
  assert_true(not exists(first_lazy_transcript), "initial lazy transcript was retained")
  assert_true(not exists(second_lazy_transcript), "cached lazy transcript was retained")

  vim.env.XDG_RUNTIME_DIR = original_xdg_runtime_dir
  vim.env.TMPDIR = original_tmpdir

  local _, published_target_ok = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  assert_true(published_target_ok, "could not publish the lazy runtime target")
  local switched_runtime_notifications, switched_runtime_ok, switched_runtime_err = capture_notifications(function()
    return talk2text.setup({ runtime_dir = runtime_dir })
  end)
  assert_true(not switched_runtime_ok, "setup changed an already-selected runtime")
  assert_true(switched_runtime_err:match("cannot change") ~= nil, "runtime change result omitted the cause")
  assert_equal(#switched_runtime_notifications, 1, "runtime change did not notify exactly once")
  assert_true(
    exists(fallback_runtime .. "/nvim-target"),
    "rejected runtime change removed the published target"
  )

  package.loaded["talk2text"] = nil
  talk2text = require("talk2text")

  local offline_runtime = root .. "/offline-runtime"
  vim.fn.mkdir(offline_runtime, "p", 448)
  local daemon_notifications, daemon_ok, daemon_err = capture_notifications(function()
    return talk2text.setup({ runtime_dir = offline_runtime })
  end)
  assert_true(not daemon_ok, "setup accepted a runtime without a live daemon")
  assert_true(daemon_err:match("daemon") ~= nil, "unavailable daemon result omitted the cause")
  assert_true(#daemon_notifications == 1, "unavailable daemon setup did not notify exactly once")
  assert_true(daemon_notifications[1][1]:match("daemon") ~= nil, "daemon notification omitted the cause")

  assert_true(talk2text.setup({ runtime_dir = runtime_dir }), "initial explicit setup failed")
  local daemon_checks = 0
  local original_check_daemon = runtime.check_daemon
  runtime.check_daemon = function()
    daemon_checks = daemon_checks + 1
    return nil, "unexpected daemon check"
  end
  local repeated_setup_notifications, repeated_setup_ok = capture_notifications(function()
    return talk2text.setup({ runtime_dir = runtime_dir })
  end)
  runtime.check_daemon = original_check_daemon
  assert_true(repeated_setup_ok, "setup rejected the selected runtime")
  assert_equal(daemon_checks, 0, "repeated setup checked the daemon again")
  assert_equal(#repeated_setup_notifications, 0, "repeated setup emitted a notification")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello, world", "tail" })
  vim.api.nvim_win_set_cursor(0, { 1, 2 })
  local word_before_punctuation = transcript_dir .. "/40.txt"
  write_file(word_before_punctuation, "again")
  assert_true(talk2text.load(40), "single-word insertion before punctuation failed")
  assert_equal(
    vim.api.nvim_buf_get_lines(0, 0, -1, false),
    { "hello again, world", "tail" },
    "single word before punctuation"
  )
  assert_equal(vim.api.nvim_win_get_cursor(0), { 1, 6 }, "cursor at inserted word")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello   world" })
  vim.api.nvim_win_set_cursor(0, { 1, 6 })
  local word_in_spaces = transcript_dir .. "/41.txt"
  write_file(word_in_spaces, "again")
  assert_true(talk2text.load(41), "single-word insertion into spaces failed")
  assert_equal(vim.api.nvim_get_current_line(), "hello again world", "single word in spaces")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "alpha beta" })
  vim.api.nvim_win_set_cursor(0, { 1, 2 })
  local hyphenated_word = transcript_dir .. "/42.txt"
  write_file(hyphenated_word, "well-known")
  assert_true(talk2text.load(42), "hyphenated single-word insertion failed")
  assert_equal(vim.api.nvim_get_current_line(), "alpha well-known beta", "hyphenated single word")

  local punctuated_transcript = transcript_dir .. "/43.txt"
  write_file(punctuated_transcript, "done.")
  assert_true(talk2text.load(43), "punctuated transcript insertion failed")
  assert_equal(vim.api.nvim_get_current_line(), "alpha well-known beta done.", "punctuated transcript")
  assert_equal(vim.api.nvim_win_get_cursor(0), { 1, 0 }, "cursor after punctuated transcript")

  local trailing_hyphen = transcript_dir .. "/44.txt"
  write_file(trailing_hyphen, "unfinished-")
  assert_true(talk2text.load(44), "trailing-hyphen transcript insertion failed")
  assert_equal(
    vim.api.nvim_get_current_line(),
    "alpha well-known beta done. unfinished-",
    "trailing-hyphen transcript"
  )

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello... world" })
  vim.api.nvim_win_set_cursor(0, { 1, 5 })
  local trailing_punctuation = transcript_dir .. "/45.txt"
  write_file(trailing_punctuation, "again")
  assert_true(talk2text.load(45), "insertion before trailing punctuation failed")
  assert_equal(vim.api.nvim_get_current_line(), "hello again... world", "trailing punctuation")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "it's ready" })
  vim.api.nvim_win_set_cursor(0, { 1, 2 })
  local internal_punctuation = transcript_dir .. "/46.txt"
  write_file(internal_punctuation, "again")
  assert_true(talk2text.load(46), "insertion after internally punctuated word failed")
  assert_equal(vim.api.nvim_get_current_line(), "it's again ready", "internal punctuation")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "    alpha" })
  vim.api.nvim_win_set_cursor(0, { 1, 2 })
  local indented_word = transcript_dir .. "/47.txt"
  write_file(indented_word, "again")
  assert_true(talk2text.load(47), "indented insertion failed")
  assert_equal(vim.api.nvim_get_current_line(), "  again alpha", "prefix indentation")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "hello   , world" })
  vim.api.nvim_win_set_cursor(0, { 1, 6 })
  local punctuation_after_whitespace = transcript_dir .. "/48.txt"
  write_file(punctuation_after_whitespace, "again")
  assert_true(talk2text.load(48), "insertion before punctuation after whitespace failed")
  assert_equal(vim.api.nvim_get_current_line(), "hello again, world", "punctuation after whitespace")

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "alpha beta" })
  vim.api.nvim_win_set_cursor(0, { 1, 2 })
  local cursor_failure = transcript_dir .. "/49.txt"
  write_file(cursor_failure, "again")
  local original_set_cursor = vim.api.nvim_win_set_cursor
  vim.api.nvim_win_set_cursor = function()
    error("cursor movement failed")
  end
  local cursor_failure_ok, cursor_failure_err = talk2text.load(49)
  vim.api.nvim_win_set_cursor = original_set_cursor
  assert_true(cursor_failure_ok, "cursor movement failure failed the load: " .. tostring(cursor_failure_err))
  assert_equal(vim.api.nvim_get_current_line(), "alpha again beta", "load after cursor movement failure")

  local unexpected_failure = transcript_dir .. "/50.txt"
  write_file(unexpected_failure, "unexpected failure")
  local original_trim = vim.trim
  vim.trim = function()
    error("unexpected trim failure")
  end
  local unexpected_notifications, unexpected_ok, unexpected_err = capture_notifications(function()
    return talk2text.load(50)
  end)
  vim.trim = original_trim
  assert_true(not unexpected_ok, "unexpected load failure returned success")
  assert_true(unexpected_err:match("unexpected trim failure") ~= nil, "unexpected load failure omitted the cause")
  assert_equal(#unexpected_notifications, 1, "unexpected load failure did not notify exactly once")
  assert_equal(unexpected_notifications[1][2], vim.log.levels.ERROR, "unexpected load failure notification level")
  assert_true(exists(unexpected_failure), "unexpected load failure removed the transcript")
  assert(uv.fs_unlink(unexpected_failure))

  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "before", "current   ", "after" })
  vim.api.nvim_win_set_cursor(0, { 2, 3 })
  local first = transcript_dir .. "/1.txt"
  write_file(first, "\nfirst\n\nthird\n")
  local first_ok, first_err = talk2text.load(1)
  assert_true(first_ok, "initial transcript load failed: " .. tostring(first_err))
  assert_equal(
    vim.api.nvim_buf_get_lines(0, 0, -1, false),
    { "before", "current first", "", "third", "after" },
    "lines appended at current line"
  )
  assert_equal(vim.api.nvim_win_get_cursor(0), { 4, 0 }, "cursor at beginning of final inserted line")
  assert_true(not exists(first), "successful load did not remove its transcript")

  local inserted = transcript_dir .. "/2.txt"
  write_file(inserted, "  fourth item  \n")
  assert_true(talk2text.load(2), "line insertion failed")
  assert_equal(
    vim.api.nvim_buf_get_lines(0, 0, -1, false),
    { "before", "current first", "", "third fourth item", "after" },
    "subsequent current-line append"
  )
  assert_equal(vim.api.nvim_win_get_cursor(0), { 4, 0 }, "cursor after single-line append")

  local empty = transcript_dir .. "/4.txt"
  write_file(empty, "")
  local before_empty = vim.api.nvim_buf_get_lines(0, 0, -1, false)
  assert_true(talk2text.load(4), "empty transcript no-op failed")
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), before_empty, "empty transcript changed the buffer")
  assert_true(not exists(empty), "empty transcript was not removed")

  local max_transcript_id = 9007199254740991
  local max_id_path = transcript_dir .. "/" .. string.format("%d", max_transcript_id) .. ".txt"
  write_file(max_id_path, "")
  assert_true(talk2text.load(max_transcript_id), "maximum safe transcript ID was rejected")

  for _, invalid_id in ipairs({ 9007199254740992, math.huge }) do
    local invalid_id_notifications, invalid_id_ok, invalid_id_err = capture_notifications(function()
      return talk2text.load(invalid_id)
    end)
    assert_true(not invalid_id_ok, "unsafe transcript ID unexpectedly loaded")
    assert_true(
      invalid_id_err:match("positive and safe integer") ~= nil,
      "unsafe transcript ID omitted the valid range"
    )
    assert_equal(#invalid_id_notifications, 1, "unsafe transcript ID did not notify exactly once")
    assert_equal(invalid_id_notifications[1][2], vim.log.levels.ERROR, "unsafe transcript ID notification level")
  end

  local retry = transcript_dir .. "/5.txt"
  local retry_notifications, retry_ok = capture_notifications(function()
    return talk2text.load(5)
  end)
  assert_true(not retry_ok, "missing transcript unexpectedly loaded")
  assert_true(#retry_notifications == 1, "missing transcript failure did not notify exactly once")
  assert_true(retry_notifications[1][1]:match("5") ~= nil, "missing transcript notification omitted its ID")
  write_file(retry, "retried text")
  local retry_success_notifications, retry_success = capture_notifications(function()
    return talk2text.load(0)
  end)
  assert_true(retry_success, "remembered transcript retry failed")
  assert_true(#retry_success_notifications == 1, "successful retry did not notify exactly once")
  assert_true(retry_success_notifications[1][1]:match("5") ~= nil, "successful retry notification omitted its ID")
  assert_true(
    retry_success_notifications[1][1]:lower():match("retry") ~= nil,
    "successful retry notification omitted its purpose"
  )
  assert_equal(retry_success_notifications[1][2], vim.log.levels.INFO, "successful retry notification level")
  assert_equal(vim.api.nvim_get_current_line(), "third fourth item retried text", "retried line")

  local blocked = transcript_dir .. "/6.txt"
  write_file(blocked, "blocked text")
  vim.api.nvim_set_option_value("modifiable", false, { buf = 0 })
  local blocked_notifications, blocked_ok = capture_notifications(function()
    return talk2text.load(6)
  end)
  assert_true(not blocked_ok, "unmodifiable buffer unexpectedly accepted a transcript")
  assert_true(#blocked_notifications == 1, "unmodifiable buffer failure did not notify exactly once")
  assert_true(blocked_notifications[1][1]:match("modifiable") ~= nil, "buffer notification omitted the cause")
  assert_true(blocked_notifications[1][1]:match("6") ~= nil, "buffer notification omitted its ID")
  assert_equal(blocked_notifications[1][2], vim.log.levels.ERROR, "buffer notification level")
  assert_true(exists(blocked), "failed buffer load removed its transcript")
  vim.api.nvim_set_option_value("modifiable", true, { buf = 0 })
  local blocked_retry_notifications, blocked_retry_ok = capture_notifications(function()
    return talk2text.load()
  end)
  assert_true(blocked_retry_ok, "retry after buffer failure failed")
  assert_true(#blocked_retry_notifications == 1, "buffer retry did not notify exactly once")
  assert_true(blocked_retry_notifications[1][1]:match("6") ~= nil, "buffer retry notification omitted its ID")
  assert_equal(blocked_retry_notifications[1][2], vim.log.levels.INFO, "buffer retry notification level")
  assert_equal(vim.api.nvim_get_current_line(), "third fourth item retried text blocked text", "retried blocked line")
  assert_true(talk2text.load(), "load without a remembered failure was not a no-op")

  local remote_blocked = transcript_dir .. "/7.txt"
  write_file(remote_blocked, "remote blocked")
  vim.api.nvim_set_option_value("modifiable", false, { buf = 0 })
  local remote_notifications, remote_result = capture_notifications(function()
    return talk2text._remote_load(7)
  end)
  assert_true(remote_result:match("^error:") ~= nil, "remote buffer failure returned success")
  assert_true(#remote_notifications == 1, "remote buffer failure did not notify exactly once")
  assert_true(remote_notifications[1][1]:match("modifiable") ~= nil, "remote notification omitted the cause")
  assert_true(remote_notifications[1][1]:match("7") ~= nil, "remote notification omitted its ID")
  vim.api.nvim_set_option_value("modifiable", true, { buf = 0 })
  local remote_retry_notifications, remote_retry_ok = capture_notifications(function()
    return talk2text.load()
  end)
  assert_true(remote_retry_ok, "remote buffer failure was not available for retry")
  assert_true(#remote_retry_notifications == 1, "remote-origin retry did not notify exactly once")
  assert_true(remote_retry_notifications[1][1]:match("7") ~= nil, "remote-origin retry notification omitted its ID")
  assert_equal(remote_retry_notifications[1][2], vim.log.levels.INFO, "remote-origin retry notification level")

  local bad_setup_notifications, bad_setup_ok, bad_setup_err = capture_notifications(function()
    return talk2text.setup({ unknown = true })
  end)
  assert_true(not bad_setup_ok, "unknown setup option was accepted")
  assert_true(bad_setup_err:match("unknown") ~= nil, "invalid setup result omitted the cause")
  assert_true(#bad_setup_notifications == 1, "invalid setup did not notify exactly once")
  assert_true(talk2text.set_default_target == nil, "default-target lifecycle was exposed as public API")

  local missing_notifications, missing_ok, missing_err = capture_notifications(function()
    return talk2text.setup({ runtime_dir = root .. "/missing-runtime" })
  end)
  assert_true(not missing_ok, "missing runtime setup unexpectedly succeeded")
  assert_true(missing_err:match("runtime directory") ~= nil, "missing runtime result omitted the cause")
  assert_true(#missing_notifications == 1, "missing runtime setup did not notify exactly once")
  assert_true(missing_notifications[1][1]:match("runtime directory") ~= nil, "runtime notification omitted the cause")

  local original_create_autocmd = vim.api.nvim_create_autocmd
  local cleanup_attempts = 0
  vim.api.nvim_create_autocmd = function(...)
    cleanup_attempts = cleanup_attempts + 1
    if cleanup_attempts == 1 then
      error("simulated cleanup registration failure")
    end
    return original_create_autocmd(...)
  end
  local cleanup_failure_notifications, cleanup_failure_ok, cleanup_failure_err = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  local cleanup_retry_notifications, cleanup_retry_ok = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  vim.api.nvim_create_autocmd = original_create_autocmd
  assert_true(not cleanup_failure_ok, "cleanup registration failure returned success")
  assert_true(
    cleanup_failure_err:match("simulated cleanup registration failure") ~= nil,
    "cleanup registration failure omitted the cause"
  )
  assert_equal(#cleanup_failure_notifications, 1, "cleanup registration failure did not notify exactly once")
  assert_true(cleanup_retry_ok, "cleanup registration retry failed")
  assert_equal(cleanup_attempts, 2, "cleanup registration was not retried")
  assert_equal(#cleanup_retry_notifications, 0, "cleanup registration retry emitted a target-switch notification")

  write_file(runtime_dir .. "/nvim-target", "replacement\n")
  local target_notifications, target_ok, target_err = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  assert_true(target_ok, "target registration failed: " .. tostring(target_err))
  assert_true(#target_notifications == 1, "actual target switch did not notify exactly once")
  assert_equal(target_notifications[1][2], vim.log.levels.INFO, "target switch notification level")

  local unchanged_notifications, unchanged_ok = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  assert_true(unchanged_ok, "unchanged target call failed")
  assert_equal(#unchanged_notifications, 0, "unchanged target emitted a switch notification")

  local target_path = runtime_dir .. "/nvim-target"
  assert(uv.fs_unlink(target_path) or not exists(target_path))
  assert(uv.fs_mkdir(target_path, 448))
  local failed_target_notifications, failed_target_ok, failed_target_err = capture_notifications(function()
    return talk2text.set_target(0)
  end)
  assert_true(not failed_target_ok, "target registration failure returned success")
  assert_true(failed_target_err:match("cannot write nvim%-target") ~= nil, "target failure omitted the cause")
  assert_equal(#failed_target_notifications, 1, "target registration failure did not notify exactly once")
  assert_equal(failed_target_notifications[1][2], vim.log.levels.ERROR, "target failure notification level")
  assert(uv.fs_rmdir(target_path))

  write_file(runtime_dir .. "/nvim-target", "replacement\n")
  local invalid_notifications, invalid_ok = capture_notifications(function()
    return talk2text.set_target(-1)
  end)
  assert_true(not invalid_ok, "negative transcript ID unexpectedly loaded")
  assert_equal(#invalid_notifications, 2, "invalid ID after target switch did not emit both notifications")
  assert_equal(invalid_notifications[1][2], vim.log.levels.INFO, "target switch notification order")
  assert_equal(invalid_notifications[2][2], vim.log.levels.ERROR, "invalid ID notification order")
  assert_true(invalid_notifications[2][1]:match("%-1") ~= nil, "invalid ID notification omitted its ID")
  local target_file = assert(io.open(runtime_dir .. "/nvim-target", "rb"))
  local target_value = assert(target_file:read("*l"))
  assert(target_file:close())
  assert_equal(target_value, target_server, "invalid ID prevented target switch")

  local startup = transcript_dir .. "/8.txt"
  write_file(startup, "startup still loads")
  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "" })
  assert(uv.fs_unlink(runtime_dir .. "/default-nvim-target") or not exists(runtime_dir .. "/default-nvim-target"))
  assert(uv.fs_mkdir(runtime_dir .. "/default-nvim-target", 448))

  local notifications = capture_notifications(function()
    talk2text._default_start(8)
  end)
  assert_true(#notifications == 1, "default registration failure was not reported exactly once")
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), { "startup still loads" }, "startup fallback load")
  assert_true(not exists(startup), "startup fallback load did not remove transcript")
  assert_equal(vim.bo.buftype, "nofile", "default editor buffer type")
  assert_equal(vim.bo.bufhidden, "wipe", "default editor buffer hidden behavior")
  assert_true(not vim.bo.modified, "default editor transcript buffer was modified")
  local mapping = vim.fn.maparg("qq", "n", false, true)
  assert_true(type(mapping) == "table" and mapping.buffer == 1, "default editor mapping is not buffer-local")
  assert_true(type(mapping.callback) == "function", "default editor mapping has no Lua callback")
  assert(uv.fs_rmdir(runtime_dir .. "/default-nvim-target"))

  local transcript_buffer = vim.api.nvim_get_current_buf()
  local transcript_window = vim.api.nvim_get_current_win()
  vim.cmd("vnew")
  local normal_buffer = vim.api.nvim_get_current_buf()
  vim.api.nvim_buf_set_lines(normal_buffer, 0, -1, false, { "modified normal buffer" })
  assert_true(vim.bo[normal_buffer].modified, "normal buffer was not modified for mapping test")
  vim.api.nvim_set_current_win(transcript_window)

  local copied_lines
  local original_setreg = vim.fn.setreg
  vim.fn.setreg = function(register, lines, register_type)
    assert_equal(register, "+", "default editor mapping register")
    assert_equal(register_type, "l", "default editor mapping register type")
    copied_lines = lines
    return 0
  end
  local mapping_ok, mapping_err = pcall(mapping.callback)
  vim.fn.setreg = original_setreg

  assert_true(mapping_ok, "default editor mapping failed: " .. tostring(mapping_err))
  assert_equal(copied_lines, { "startup still loads" }, "default editor mapping clipboard content")
  assert_true(not vim.api.nvim_buf_is_valid(transcript_buffer), "closed transcript buffer was not wiped")
  assert_equal(vim.api.nvim_get_current_buf(), normal_buffer, "default editor mapping closed another buffer")
  assert_true(vim.bo[normal_buffer].modified, "default editor mapping discarded another modified buffer")

  io.stdout:write("plugin tests passed\n")
  io.stdout:flush()
end

local ok, err = xpcall(run, debug.traceback)
if not ok then
  vim.api.nvim_err_writeln(err)
  os.exit(1)
end
os.exit(0)
