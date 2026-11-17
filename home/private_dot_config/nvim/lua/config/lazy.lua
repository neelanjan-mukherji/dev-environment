local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"
if not vim.uv.fs_stat(lazypath) then
    vim.fn.system({ "git", "clone", "--filter=blob:none", "https://github.com/folke/lazy.nvim.git", "--branch=stable", lazypath })
    -- Pin lazy.nvim itself to lazy-lock.json like every other plugin: `Lazy restore`
    -- cannot roll back the plugin manager while it is running.
    local ok, lock = pcall(function()
        return vim.json.decode(table.concat(vim.fn.readfile(vim.fn.stdpath("config") .. "/lazy-lock.json"), "\n"))
    end)
    local commit = ok and lock and lock["lazy.nvim"] and lock["lazy.nvim"].commit
    if commit then
        vim.fn.system({ "git", "-C", lazypath, "checkout", "--quiet", commit })
    end
end
vim.opt.rtp:prepend(lazypath)
require("lazy").setup("plugins", { checker = { enabled = true, notify = false }, change_detection = { notify = false } })
