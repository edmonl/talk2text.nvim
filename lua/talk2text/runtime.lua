local ffi = require('ffi')
local uv = vim.uv

ffi.cdef([[
int flock(int fd, int operation);
]])

local M = {}

local function flock(fd, operation)
  while ffi.C.flock(fd, operation) ~= 0 do
    local errno = ffi.errno()
    if errno ~= 4 then -- Linux EINTR: retry an interrupted system call.
      return nil, ('flock failed with errno %d'):format(errno)
    end
  end
  return true
end

---Validate a runtime directory and distinguish an absent path from other failures.
---@param path any
---@return boolean|nil exists True for a valid directory, false when absent, and nil for other failures.
---@return string|nil err
local function validate_runtime_dir(path)
  if type(path) ~= 'string' or path == '' then
    return nil, 'runtime directory must be a non-empty string'
  end
  if path:sub(1, 1) ~= '/' then
    return nil, 'runtime directory must be absolute'
  end

  local stat, err, err_name = uv.fs_stat(path)
  if not stat then
    if err_name == 'ENOENT' then
      return false, ('runtime directory is unavailable: %s'):format(err)
    end
    return nil, ('runtime directory is unavailable: %s'):format(err)
  end
  if stat.type ~= 'directory' then
    return nil, 'runtime directory is not a directory'
  end
  return true
end

---Resolve and validate the talk2text runtime directory.
---@param configured string|nil
---@return string|nil path
---@return string|nil err
function M.resolve(configured)
  if configured ~= nil then
    local exists, err = validate_runtime_dir(configured)
    if not exists then
      return nil, err
    end
    return configured
  end

  local xdg_runtime_dir = vim.env.XDG_RUNTIME_DIR
  if xdg_runtime_dir and xdg_runtime_dir ~= '' then
    local path = xdg_runtime_dir .. '/talk2text'
    local exists, err = validate_runtime_dir(path)
    if exists then
      return path
    end
    if exists == nil then
      return nil, err
    end
  end

  local uid = uv.getuid()
  local tmpdir = vim.env.TMPDIR
  if tmpdir and tmpdir ~= '' then
    local path = ('%s/run-%d/talk2text'):format(tmpdir, uid)
    local exists, err = validate_runtime_dir(path)
    if exists then
      return path
    end
    if exists == nil then
      return nil, err
    end
  end

  local path = ('/tmp/run-%d/talk2text'):format(uid)
  local exists, err = validate_runtime_dir(path)
  if exists then
    return path
  end
  return nil, err
end

---Check that the daemon socket accepts a connection.
---@param runtime_dir string
---@return true|nil ok
---@return string|nil err
function M.check_daemon(runtime_dir)
  local socket_path = runtime_dir .. '/daemon.sock'
  local connected, channel = pcall(vim.fn.sockconnect, 'pipe', socket_path, { rpc = false })
  if not connected or channel == 0 then
    return nil, 'talk2text daemon is unavailable: ' .. socket_path
  end
  pcall(vim.fn.chanclose, channel)
  return true
end

---Run a callback while holding the runtime directory's exclusive advisory lock.
---@generic T, U
---@param runtime_dir string
---@param callback (fun(): T)|(fun(): T, U)
---@return T|nil result
---@return U|string|nil second_or_err
function M.with_lock(runtime_dir, callback)
  local fd, open_err = uv.fs_open(runtime_dir, 'r', 0)
  if not fd then
    return nil, ('cannot open runtime directory for locking: %s'):format(open_err)
  end

  local locked, lock_err = flock(fd, 2) -- Linux LOCK_EX: acquire an exclusive lock.
  if not locked then
    uv.fs_close(fd)
    return nil, lock_err
  end

  local called, result, second = pcall(callback)
  local unlocked, unlock_err = flock(fd, 8) -- Linux LOCK_UN: release the lock.
  local closed, close_err = uv.fs_close(fd)

  if not called then
    return nil, result
  end
  if not unlocked then
    return nil, unlock_err
  end
  if not closed then
    return nil, ('cannot close runtime directory lock: %s'):format(close_err)
  end

  return result, second
end

return M
