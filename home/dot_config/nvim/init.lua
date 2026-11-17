vim.g.mapleader = " "
vim.g.maplocalleader = " "
vim.opt.number = true
vim.opt.relativenumber = true
vim.opt.cursorline = true
vim.opt.termguicolors = true
vim.opt.signcolumn = "yes"
vim.opt.updatetime = 250
vim.opt.timeoutlen = 400
vim.opt.splitright = true
vim.opt.splitbelow = true
vim.opt.expandtab = true
vim.opt.shiftwidth = 4
vim.opt.tabstop = 4
vim.opt.smartindent = true
vim.opt.wrap = false
vim.opt.ignorecase = true
vim.opt.smartcase = true
vim.opt.hlsearch = true
vim.opt.incsearch = true
vim.opt.mouse = "a"
vim.opt.clipboard = "unnamedplus"
vim.opt.undofile = true
vim.opt.swapfile = false
vim.opt.scrolloff = 8
vim.opt.sidescrolloff = 8
vim.api.nvim_create_autocmd("TextYankPost", { callback = function() vim.highlight.on_yank() end })
require("config.lazy")
require("config.options")
require("config.keymaps")

-- ==========================================
-- Clipboard
-- ==========================================
-- Neovim auto-detects pbcopy, wl-copy and xclip. Under WSL, use the Windows
-- clipboard directly.
if vim.fn.has("wsl") == 1 then
    vim.g.clipboard = {
        name = "Windows Clipboard",
        copy = { ["+"] = { "clip.exe" }, ["*"] = { "clip.exe" } },
        paste = {
            ["+"] = { "powershell.exe", "-NoProfile", "-Command", "Get-Clipboard -Raw" },
            ["*"] = { "powershell.exe", "-NoProfile", "-Command", "Get-Clipboard -Raw" },
        },
        cache_enabled = 0,
    }
end
