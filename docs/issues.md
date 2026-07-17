# Major

# Minor

## 1. Supported environment and acceptance coverage are not recorded

State the supported Linux, Bash, and Neovim baseline. Extend the test checklist to cover stale targets, target replacement during stale cleanup, a reachable target whose load fails, successful and failed transcript cleanup, missing required hooks, `blank` and `short`, and transcript paths containing spaces.

**Recommended resolution:** Initial support is Linux with Bash 4+ and Neovim 0.9+. The Neovim client and terminal hooks are required; notification and focus hooks are optional. Users configure hooks with environment variables or in a copied command; Sway and Alacritty are illustrative only. The manual checklist should cover the listed cleanup, fallback, failure, kind, and whitespace-path cases before release.
