local runtime = require("talk2text.runtime")
local target = require("talk2text.target")
local uv = vim.uv or vim.loop

local M = {}
local config = { runtime_dir = nil }
local failed_path

local function notify_error(message)
  vim.notify("talk2text: " .. tostring(message), vim.log.levels.ERROR)
end

local function remember_failure(path, explicit)
  if explicit then
    failed_path = path
  end
end

local function read_file(path)
  local stat, stat_err = uv.fs_stat(path)
  if not stat then
    return nil, ("cannot inspect transcript: %s"):format(stat_err)
  end
  if stat.type ~= "file" then
    return nil, "transcript is not a regular file"
  end

  local fd, open_err = uv.fs_open(path, "r", 0)
  if not fd then
    return nil, ("cannot open transcript: %s"):format(open_err)
  end

  local chunks = {}
  local offset = 0
  while true do
    local chunk, read_err = uv.fs_read(fd, 65536, offset)
    if chunk == nil then
      uv.fs_close(fd)
      return nil, ("cannot read transcript: %s"):format(read_err)
    end
    if chunk == "" then
      break
    end
    chunks[#chunks + 1] = chunk
    offset = offset + #chunk
  end

  local closed, close_err = uv.fs_close(fd)
  if not closed then
    return nil, ("cannot close transcript: %s"):format(close_err)
  end
  return table.concat(chunks)
end

local function split_lines(contents)
  if contents == "" then
    return {}
  end

  local lines = {}
  local start = 1
  while true do
    local newline = contents:find("\n", start, true)
    if not newline then
      lines[#lines + 1] = contents:sub(start)
      break
    end

    lines[#lines + 1] = contents:sub(start, newline - 1)
    start = newline + 1
    if start > #contents then
      break
    end
  end
  return lines
end

local function append_lines(lines)
  if #lines == 0 then
    return true, false
  end

  local buffer = vim.api.nvim_get_current_buf()
  local existing = vim.api.nvim_buf_get_lines(buffer, 0, -1, false)
  local first = 0
  local last = -1
  if not (#existing == 1 and existing[1] == "") then
    first = -1
  end

  local ok, err = pcall(vim.api.nvim_buf_set_lines, buffer, first, last, false, lines)
  if not ok then
    return nil, err
  end
  return true, true
end

local function register_cleanup(runtime_dir, filename, servername)
  vim.api.nvim_create_autocmd("VimLeavePre", {
    once = true,
    desc = "Remove the talk2text Neovim target",
    callback = function()
      local ok, err = target.delete_if_matches(runtime_dir, filename, servername)
      if not ok then
        pcall(notify_error, err)
      end
    end,
  })
end

local function servername()
  local name = vim.v.servername
  if name == "" then
    local ok, result = pcall(vim.fn.serverstart)
    if not ok or type(result) ~= "string" or result == "" then
      error("cannot start a Neovim server: " .. tostring(result), 3)
    end
    name = result
  end
  return name
end

local function set_target_file(filename)
  local runtime_dir, runtime_err = runtime.resolve(config.runtime_dir)
  if not runtime_dir then
    error(runtime_err, 3)
  end

  local name = servername()
  local ok, write_err = target.write(runtime_dir, filename, name)
  if not ok then
    error(("cannot write %s: %s"):format(filename, write_err), 3)
  end
  register_cleanup(runtime_dir, filename, name)
  return true
end

local function configure_default_mapping(buffer)
  vim.keymap.set("n", "qq", function()
    local lines = vim.api.nvim_buf_get_lines(buffer, 0, -1, false)
    local ok, result = pcall(vim.fn.setreg, "+", lines, "l")
    if not ok or result ~= 0 then
      notify_error(ok and "cannot copy the transcript to the clipboard" or result)
      return
    end
    vim.cmd("qa!")
  end, {
    buffer = buffer,
    desc = "Copy the talk2text transcript and quit",
    silent = true,
  })
end

---Configure talk2text.nvim.
---@param opts? {runtime_dir?: string}
function M.setup(opts)
  opts = opts or {}
  if type(opts) ~= "table" then
    error("talk2text.setup expects a table", 2)
  end
  for key in pairs(opts) do
    if key ~= "runtime_dir" then
      error("unknown talk2text option: " .. tostring(key), 2)
    end
  end
  if opts.runtime_dir ~= nil and (type(opts.runtime_dir) ~= "string" or opts.runtime_dir == "") then
    error("talk2text runtime_dir must be a non-empty string", 2)
  end
  config.runtime_dir = opts.runtime_dir
end

---Make the current Neovim instance the explicit talk2text target.
---@return true
function M.set_target()
  return set_target_file("nvim-target")
end

---Make the current Neovim instance the default talk2text editor target.
---@return true
function M.set_default_target()
  return set_target_file("default-nvim-target")
end

---Load a transcript into the current buffer, or retry the last failed path.
---@param path? string
---@return true|nil ok
---@return string|nil err
function M.load(path)
  local explicit = path ~= nil
  if not explicit then
    path = failed_path
    if path == nil then
      return true
    end
  elseif type(path) ~= "string" or path == "" then
    return nil, "transcript path must be a non-empty string"
  end

  local contents, read_err = read_file(path)
  if contents == nil then
    remember_failure(path, explicit)
    return nil, read_err
  end

  local appended, changed_or_err = append_lines(split_lines(contents))
  if not appended then
    remember_failure(path, explicit)
    return nil, tostring(changed_or_err)
  end

  local removed, remove_err = uv.fs_unlink(path)
  if not removed then
    if changed_or_err then
      failed_path = nil
    else
      remember_failure(path, explicit)
    end
    return nil, ("cannot remove transcript: %s"):format(remove_err)
  end

  failed_path = nil
  return true
end

---Internal RPC adapter used by talk2text-nvim.
---@param path string
---@return string
function M._remote_load(path)
  local call = { pcall(M.load, path) }
  if not call[1] then
    return "talk2text-error:" .. tostring(call[2])
  end
  if call[2] then
    return "talk2text-ok"
  end
  return "talk2text-error:" .. tostring(call[3])
end

---Internal startup adapter used by talk2text-nvim's default editor.
---@param path string
function M._default_start(path)
  local buffer = vim.api.nvim_get_current_buf()
  local registered, register_err = pcall(M.set_default_target)
  if not registered then
    notify_error(register_err)
  end

  local loaded, load_err = M.load(path)
  if not loaded then
    notify_error(load_err)
  end
  configure_default_mapping(buffer)
end

return M
