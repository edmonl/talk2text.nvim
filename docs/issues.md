# Major

# Minor

## 1. Supported environment and acceptance coverage are not recorded

State the supported Linux, Bash, and Neovim baseline. Extend the test checklist to cover stale targets, target replacement during stale cleanup, a reachable target whose load fails, successful and failed transcript cleanup, missing required hooks, `blank` and `short`, and transcript paths containing spaces.

**Recommended resolution:** Initial support is Linux with Bash 4+ and Neovim 0.10+. The Neovim command is required; the launch command, notification hook, and focus hook are optional. Users configure hooks with environment variables or in a copied command. The manual checklist should cover the listed cleanup, fallback, failure, kind, and whitespace-path cases before release.

## 2. Numeric transcript IDs are not bounded consistently

The Lua API accepts non-finite and oversized numeric values that satisfy the current integer check but format as a different filename under LuaJIT. The Go command accepts signed 64-bit IDs, while the Bash command accepts arbitrarily long decimal strings, so the three boundaries do not share one numeric domain.

**Recommended resolution:** Define one supported ID range and validate it consistently in Lua, Bash, and Go, or carry canonical decimal IDs as strings across the RPC boundary.
