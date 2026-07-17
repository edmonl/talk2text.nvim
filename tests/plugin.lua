local uv = vim.uv or vim.loop
local root = assert(vim.env.TALK2TEXT_TEST_DIR, "TALK2TEXT_TEST_DIR is required")
local runtime_dir = root .. "/runtime"
local transcript_dir = runtime_dir .. "/transcripts"

local function run()
  vim.fn.mkdir(transcript_dir, "p", 448)

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

  local talk2text = require("talk2text")
  talk2text.setup({ runtime_dir = runtime_dir })

  local first = transcript_dir .. "/first.txt"
  write_file(first, "first\n\nthird\n")
  local first_ok, first_err = talk2text.load(first)
  assert_true(first_ok, "initial transcript load failed: " .. tostring(first_err))
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), { "first", "", "third" }, "initial lines")
  assert_true(not exists(first), "successful load did not remove its transcript")

  local appended = transcript_dir .. "/appended.txt"
  write_file(appended, "fourth\n")
  assert_true(talk2text.load(appended), "append failed")
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), { "first", "", "third", "fourth" }, "appended lines")

  local blank_line = transcript_dir .. "/blank-line.txt"
  write_file(blank_line, "\n")
  assert_true(talk2text.load(blank_line), "intentional blank line failed")
  assert_equal(
    vim.api.nvim_buf_get_lines(0, 0, -1, false),
    { "first", "", "third", "fourth", "" },
    "intentional blank line"
  )

  local empty = transcript_dir .. "/empty.txt"
  write_file(empty, "")
  local before_empty = vim.api.nvim_buf_get_lines(0, 0, -1, false)
  assert_true(talk2text.load(empty), "empty transcript no-op failed")
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), before_empty, "empty transcript changed the buffer")
  assert_true(not exists(empty), "empty transcript was not removed")

  local retry = transcript_dir .. "/retry.txt"
  local retry_ok = talk2text.load(retry)
  assert_true(not retry_ok, "missing transcript unexpectedly loaded")
  write_file(retry, "retried")
  assert_true(talk2text.load(), "remembered transcript retry failed")
  assert_equal(vim.api.nvim_buf_get_lines(0, -1, -1, false), {}, "invalid line range sanity check")
  assert_equal(vim.api.nvim_buf_get_lines(0, -2, -1, false), { "retried" }, "retried line")

  local blocked = transcript_dir .. "/blocked.txt"
  write_file(blocked, "blocked")
  vim.api.nvim_set_option_value("modifiable", false, { buf = 0 })
  local blocked_ok = talk2text.load(blocked)
  assert_true(not blocked_ok, "unmodifiable buffer unexpectedly accepted a transcript")
  assert_true(exists(blocked), "failed buffer load removed its transcript")
  vim.api.nvim_set_option_value("modifiable", true, { buf = 0 })
  assert_true(talk2text.load(), "retry after buffer failure failed")
  assert_equal(vim.api.nvim_buf_get_lines(0, -2, -1, false), { "blocked" }, "retried blocked line")
  assert_true(talk2text.load(), "load without a remembered failure was not a no-op")

  local bad_setup_ok = pcall(talk2text.setup, { unknown = true })
  assert_true(not bad_setup_ok, "unknown setup option was accepted")

  local startup = transcript_dir .. "/startup.txt"
  write_file(startup, "startup still loads")
  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "" })
  talk2text.setup({ runtime_dir = root .. "/missing-runtime" })
  local notifications = {}
  local original_notify = vim.notify
  vim.notify = function(message, level)
    notifications[#notifications + 1] = { message, level }
  end
  talk2text._default_start(startup)
  vim.notify = original_notify
  assert_true(#notifications == 1, "default registration failure was not reported exactly once")
  assert_equal(vim.api.nvim_buf_get_lines(0, 0, -1, false), { "startup still loads" }, "startup fallback load")
  assert_true(not exists(startup), "startup fallback load did not remove transcript")
  local mapping = vim.fn.maparg("qq", "n", false, true)
  assert_true(type(mapping) == "table" and mapping.buffer == 1, "default editor mapping is not buffer-local")

  io.stdout:write("plugin tests passed\n")
  io.stdout:flush()
end

local ok, err = xpcall(run, debug.traceback)
if not ok then
  vim.api.nvim_err_writeln(err)
  os.exit(1)
end
os.exit(0)
