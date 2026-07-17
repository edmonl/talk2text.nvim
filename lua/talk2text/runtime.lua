local ffi = require("ffi")
local uv = vim.uv or vim.loop

ffi.cdef([[
int flock(int fd, int operation);
]])

local LOCK_EX = 2
local LOCK_UN = 8
local EINTR = 4
local unpack_values = table.unpack or unpack

local M = {}

local function flock(fd, operation)
  while ffi.C.flock(fd, operation) ~= 0 do
    local errno = ffi.errno()
    if errno ~= EINTR then
      return nil, ("flock failed with errno %d"):format(errno)
    end
  end
  return true
end

local function validate_runtime_dir(path)
  if type(path) ~= "string" or path == "" then
    return nil, "runtime directory must be a non-empty string"
  end
  if path:sub(1, 1) ~= "/" then
    return nil, "runtime directory must be absolute"
  end

  local stat, err = uv.fs_stat(path)
  if not stat then
    return nil, ("runtime directory is unavailable: %s"):format(err)
  end
  if stat.type ~= "directory" then
    return nil, "runtime directory is not a directory"
  end
  return path
end

---Resolve and validate the talk2text runtime directory.
---@param configured string|nil
---@return string|nil path
---@return string|nil err
function M.resolve(configured)
  if configured ~= nil then
    return validate_runtime_dir(configured)
  end

  local xdg_runtime_dir = vim.env.XDG_RUNTIME_DIR
  if xdg_runtime_dir and xdg_runtime_dir ~= "" then
    return validate_runtime_dir(xdg_runtime_dir .. "/talk2text")
  end

  local uid = uv.getuid()
  local tmpdir = vim.env.TMPDIR
  if tmpdir and tmpdir ~= "" then
    return validate_runtime_dir(("%s/run-%d/talk2text"):format(tmpdir, uid))
  end

  return validate_runtime_dir(("/tmp/run-%d/talk2text"):format(uid))
end

---Run a callback while holding the runtime directory's exclusive advisory lock.
---@generic T
---@param runtime_dir string
---@param callback fun(): T, string|nil
---@return T|nil result
---@return string|nil err
function M.with_lock(runtime_dir, callback)
  local fd, open_err = uv.fs_open(runtime_dir, "r", 0)
  if not fd then
    return nil, ("cannot open runtime directory for locking: %s"):format(open_err)
  end

  local locked, lock_err = flock(fd, LOCK_EX)
  if not locked then
    uv.fs_close(fd)
    return nil, lock_err
  end

  local call = { n = 0 }
  local function pack(...)
    call.n = select("#", ...)
    for index = 1, call.n do
      call[index] = select(index, ...)
    end
  end
  pack(pcall(callback))
  local unlocked, unlock_err = flock(fd, LOCK_UN)
  local closed, close_err = uv.fs_close(fd)

  if not call[1] then
    return nil, call[2]
  end
  if not unlocked then
    return nil, unlock_err
  end
  if not closed then
    return nil, ("cannot close runtime directory lock: %s"):format(close_err)
  end

  return unpack_values(call, 2, call.n)
end

return M
