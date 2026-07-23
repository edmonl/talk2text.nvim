local runtime = require('talk2text.runtime')
local target = require('talk2text.target')
local uv = vim.uv

local M = {}
local config = { runtime_dir = nil }
local max_transcript_id = 9007199254740991 -- Maximum safe integer for LuaJIT numbers.
local failed_id -- Last explicitly requested transcript ID that failed, for retry.
local cleanup_servers = {} -- Target filenames mapped to servers with registered exit cleanup.

local function notify(message, level, id)
  local prefix = 'Talk2text: '
  if id ~= nil then
    prefix = ('Talk2text transcript %s: '):format(tostring(id))
  end
  vim.notify(prefix .. tostring(message), level)
end

local function notify_error(message, id)
  notify(message, vim.log.levels.ERROR, id)
end

local function get_runtime_dir()
  if config.runtime_dir ~= nil then
    return config.runtime_dir
  end

  local runtime_dir, err = runtime.resolve()
  if runtime_dir then
    config.runtime_dir = runtime_dir
  end
  return runtime_dir, err
end

local function read_transcript_file(path)
  local stat, stat_err = uv.fs_stat(path)
  if not stat then
    return nil, ('cannot inspect transcript: %s'):format(stat_err)
  end
  if stat.type ~= 'file' then
    return nil, 'transcript is not a regular file'
  end

  local file, open_err = io.open(path, 'rb')
  if not file then
    return nil, ('cannot open transcript: %s'):format(open_err)
  end

  local contents, read_err = file:read('*a')
  local closed, close_err = file:close()
  if contents == nil then
    return nil, ('cannot read transcript: %s'):format(read_err)
  end
  if not closed then
    return nil, ('cannot close transcript: %s'):format(close_err)
  end
  return contents
end

---Append text to the cursor's current line.
---@param text string
---@return boolean ok
---@return boolean|string changed_or_err Whether the buffer changed, or the error on failure.
local function append_text_at_cursor_line(text)
  local lines = vim.split(text, '\n', { plain = true, trimempty = true })
  if #lines == 0 then
    return true, false
  end

  -- Command-line startup can report row zero before the first buffer window is fully initialized.
  local current_line = vim.api.nvim_get_current_line()
  if current_line ~= '' then
    lines[1] = current_line:gsub(' +$', '') .. ' ' .. lines[1]
  end

  local row = math.max(vim.api.nvim_win_get_cursor(0)[1], 1)
  local ok, err = pcall(vim.api.nvim_buf_set_lines, 0, row - 1, row, true, lines)
  if not ok then
    return false, tostring(err)
  end
  pcall(vim.api.nvim_win_set_cursor, 0, { row + #lines - 1, 0 })
  return true, true
end

---Insert a word after the whitespace-delimited word under the cursor.
---@param word string
---@return boolean ok
---@return true|string changed_or_err True on success, or the error on failure.
local function insert_word_at_cursor(word)
  local cursor = vim.api.nvim_win_get_cursor(0)
  local line = vim.api.nvim_get_current_line()
  local position = math.min(cursor[2] + 1, #line + 1)
  local prefix = line:sub(1, position - 1)
  local suffix = line:sub(position)
  local token, whitespace = suffix:match('^(%S*)(%s*)')
  local word_tail, punctuation = token:match('^(.-)(%p*)$')
  suffix = suffix:sub(#token + #whitespace + 1)
  -- line: prefix, word_tail, punctuation, whitespace, suffix

  if word_tail ~= '' then
    prefix = prefix .. word_tail .. ' '
    suffix = (punctuation .. whitespace .. suffix):gsub('^%s+', ' ')
  elseif punctuation ~= '' then
    if prefix:match('%S$') then
      prefix = prefix .. ' '
    end
    suffix = punctuation .. whitespace .. suffix
  else
    if prefix:match('%S$') then
      prefix = prefix .. ' '
    end
    if suffix:match('^[^%s%p]') then
      suffix = ' ' .. suffix
    end
  end

  local ok, err = pcall(vim.api.nvim_set_current_line, prefix .. word .. suffix)
  if not ok then
    return false, tostring(err)
  end
  pcall(vim.api.nvim_win_set_cursor, 0, { cursor[1], #prefix })
  return true, true
end

local function register_cleanup(runtime_dir, filename, servername)
  if cleanup_servers[filename] == servername then
    return
  end
  vim.api.nvim_create_autocmd('VimLeavePre', {
    once = true,
    desc = 'Remove Talk2text Neovim Target',
    callback = function()
      target.delete_if_matches(runtime_dir, filename, servername)
    end,
  })
  cleanup_servers[filename] = servername
end

local function servername()
  if vim.v.servername ~= '' then
    return vim.v.servername
  end

  local ok, result = pcall(vim.fn.serverstart)
  if ok then
    return result
  end
  return nil, 'cannot start a server: ' .. tostring(result)
end

---Publish the current Neovim server as a target and register its exit cleanup.
---@param filename string
---@return boolean ok
---@return boolean|string changed_or_err Whether the target changed, or the error on failure.
local function set_target_file(filename)
  local runtime_dir, runtime_err = get_runtime_dir()
  if not runtime_dir then
    return false, tostring(runtime_err)
  end

  local name, server_err = servername()
  if not name then
    return false, tostring(server_err)
  end
  local ok, changed_or_err = target.claim(runtime_dir, filename, name)
  if not ok then
    return false, ('cannot write %s: %s'):format(filename, changed_or_err)
  end
  register_cleanup(runtime_dir, filename, name)
  return true, changed_or_err == true
end

---Set a target file while converting unexpected Lua errors into returned errors.
---@param filename string
---@return boolean ok
---@return boolean|string changed_or_err Whether the target changed, or the error on failure.
local function try_set_target(filename)
  local called, result, changed_or_err = pcall(set_target_file, filename)
  if not called then
    return false, tostring(result)
  end
  if not result then
    return false, tostring(changed_or_err)
  end
  return true, changed_or_err
end

local function configure_default_mapping(buffer)
  vim.keymap.set('n', 'qq', function()
    local lines = vim.api.nvim_buf_get_lines(buffer, 0, -1, false)
    local ok, result = pcall(vim.fn.setreg, '+', lines, 'l')
    if not ok or result ~= 0 then
      notify_error(ok and 'cannot copy the transcript to the clipboard' or result)
      return
    end
    vim.cmd('q')
  end, {
    buffer = buffer,
    desc = 'Copy Talk2text Transcript And Close',
    silent = true,
  })
end

local function setup(opts)
  opts = opts or {}
  if type(opts) ~= 'table' then
    return false, 'setup expects a table'
  end
  for key in pairs(opts) do
    if key ~= 'runtime_dir' then
      return false, 'unknown option: ' .. tostring(key)
    end
  end
  local runtime_dir, runtime_err = runtime.resolve(opts.runtime_dir)
  if not runtime_dir then
    return false, runtime_err
  end
  if config.runtime_dir == runtime_dir then
    return true
  end
  if config.runtime_dir ~= nil then
    return false, ('cannot change already resolved runtime directory %s to %s')
      :format(config.runtime_dir, runtime_dir)
  end
  local daemon_ok, daemon_err = runtime.check_daemon(runtime_dir)
  if not daemon_ok then
    return false, daemon_err
  end
  config.runtime_dir = runtime_dir
  return true
end

---Configure talk2text.nvim.
---@param opts? {runtime_dir?: string}
---@return boolean ok
---@return string|nil err
function M.setup(opts)
  local ok, err = setup(opts)
  if not ok then
    notify_error(err)
    return false, err
  end
  return true
end

---Load a transcript by ID, or retry the last failed ID.
---@param id any A positive safe integer, nil, or zero; other values produce a validation failure.
---@return boolean ok
---@return string|nil err
---@return any reported_id The failed or successfully retried ID to report; nil otherwise.
local function load(id)
  if id == 0 then
    id = nil
  end

  local retried_id
  if id == nil then
    id = failed_id
    if id == nil then
      return true
    end
    retried_id = id
  elseif type(id) ~= 'number' or id ~= math.floor(id) or id < 1 or id > max_transcript_id then
    return false, ('transcript ID must be a positive and safe integer'), id
  end

  local runtime_dir, runtime_err = get_runtime_dir()
  if not runtime_dir then
    failed_id = id
    return false, runtime_err, id
  end
  local path = ('%s/transcripts/%d'):format(runtime_dir, id)

  local contents, read_err = read_transcript_file(path)
  if contents == nil then
    failed_id = id
    return false, read_err, id
  end

  local transcript = vim.trim(contents)
  local loaded, changed_or_err
  if transcript ~= '' and not transcript:find('%s') and not transcript:match('%p$') then
    loaded, changed_or_err = insert_word_at_cursor(transcript)
  else
    loaded, changed_or_err = append_text_at_cursor_line(transcript)
  end
  if not loaded then
    failed_id = id
    return false, tostring(changed_or_err), id
  end

  local removed, remove_err = uv.fs_unlink(path)
  if not removed then
    if changed_or_err then
      failed_id = nil
    else
      failed_id = id
    end
    return false, ('cannot remove transcript: %s'):format(remove_err), id
  end

  failed_id = nil
  return true, nil, retried_id
end

---Load a transcript by clip ID, or retry the last failed ID.
---@param id? integer
---@return boolean ok
---@return string|nil err
function M.load(id)
  local called, result, err, retried_id = pcall(load, id)
  if not called then
    err = tostring(result)
    notify_error(err, id)
    return false, err
  end
  if not result then
    notify_error(err, retried_id)
  elseif retried_id ~= nil then
    notify('Loaded successfully after retry', vim.log.levels.INFO, retried_id)
  end
  return result, err
end

---Make this Neovim the explicit target, then load or retry a transcript.
---@param id? integer
---@return boolean ok
---@return string|nil err
function M.set_target(id)
  local ok, changed_or_err = try_set_target('nvim-target')
  if not ok then
    notify_error(changed_or_err)
    return false, tostring(changed_or_err)
  end
  if changed_or_err then
    notify('This Neovim is now Talk2text target', vim.log.levels.INFO)
  end
  return M.load(id)
end

---Configure this Neovim as a plugin-managed default target and load a transcript.
---@param id integer
function M.start_default_target(id)
  local buffer = vim.api.nvim_get_current_buf()
  vim.api.nvim_set_option_value('buftype', 'nofile', { buf = buffer })
  vim.api.nvim_set_option_value('bufhidden', 'wipe', { buf = buffer })

  local ok, err = try_set_target('default-nvim-target')
  if not ok then
    notify_error(err)
  end

  M.load(id)
  configure_default_mapping(buffer)
end

return M
