local runtime = require('talk2text.runtime')
local uv = vim.uv

local M = {}
local temporary_sequence = 0

local function read_first_line(path)
  local fd, open_err, open_err_name = uv.fs_open(path, 'r', 0)
  if not fd then
    if open_err_name == 'ENOENT' then
      return false, ''
    end
    return nil, open_err
  end
  local contents, read_err = uv.fs_read(fd, 8192, 0)
  local closed, close_err = uv.fs_close(fd)
  if not contents then
    return nil, read_err
  end
  if not closed then
    return nil, close_err
  end
  local line = contents:match('^[^\n]*')
  return true, (line:gsub('^%s+', ''):gsub('%s+$', ''))
end

local function write_all(fd, contents)
  local offset = 0
  while offset < #contents do
    local written, err = uv.fs_write(fd, contents:sub(offset + 1), offset)
    if not written then
      return nil, err
    end
    offset = offset + written
  end
  return true
end

local function atomic_write(path, contents)
  local temporary
  local fd
  local open_err
  local open_err_name
  for _ = 1, 100 do
    temporary_sequence = temporary_sequence + 1
    temporary = ('%s.%d.%d.tmp'):format(path, vim.fn.getpid(), temporary_sequence)
    fd, open_err, open_err_name = uv.fs_open(temporary, 'wx', 384) -- 0600: owner read/write only.
    if fd or open_err_name ~= 'EEXIST' then
      break
    end
  end
  if not fd then
    return nil, open_err
  end

  local written, write_err = write_all(fd, contents)
  local closed, close_err = uv.fs_close(fd)
  if not written then
    uv.fs_unlink(temporary)
    return nil, write_err
  end
  if not closed then
    uv.fs_unlink(temporary)
    return nil, close_err
  end

  local renamed, rename_err = uv.fs_rename(temporary, path)
  if not renamed then
    uv.fs_unlink(temporary)
    return nil, rename_err
  end
  return true
end

---Make a Neovim server the target and report whether the target changed.
---@param runtime_dir string
---@param filename string
---@param servername string
---@return boolean|nil ok
---@return boolean|string|nil changed_or_err Whether the target changed, or an error on failure.
function M.claim(runtime_dir, filename, servername)
  return runtime.with_lock(runtime_dir, function()
    local exists, value_or_err = read_first_line(runtime_dir .. '/' .. filename)
    if exists == nil then
      return nil, value_or_err
    end
    if exists and value_or_err == servername then
      return true, false
    end

    local written, write_err = atomic_write(runtime_dir .. '/' .. filename, servername .. '\n')
    if not written then
      return nil, write_err
    end
    return true, true
  end)
end

---Delete a target file only when it still identifies the expected server.
---@param runtime_dir string
---@param filename string
---@param servername string
---@return boolean|nil ok
---@return string|nil err
function M.delete_if_matches(runtime_dir, filename, servername)
  return runtime.with_lock(runtime_dir, function()
    local exists, value_or_err = read_first_line(runtime_dir .. '/' .. filename)
    if exists == nil then
      return nil, value_or_err
    end
    if not exists or value_or_err ~= servername then
      return true
    end

    local removed, remove_err = uv.fs_unlink(runtime_dir .. '/' .. filename)
    if not removed then
      return nil, remove_err
    end
    return true
  end)
end

return M
