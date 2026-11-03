return {
 { "nvim-treesitter/nvim-treesitter", build = ":TSUpdate", config = function()
   require("nvim-treesitter.configs").setup({ ensure_installed = { "bash","c","cpp","css","dockerfile","go","html","java","javascript","json","lua","markdown","python","regex","toml","typescript","vim","vimdoc","yaml","xml" }, highlight = { enable = true }, indent = { enable = true } })
 end },
 { "neovim/nvim-lspconfig", dependencies = { "williamboman/mason.nvim", "williamboman/mason-lspconfig.nvim" }, config = function()
   require("mason").setup()
   require("mason-lspconfig").setup({ ensure_installed = { "lua_ls","bashls","jsonls","yamlls" } })
   local l = require("lspconfig"); l.lua_ls.setup({}); l.bashls.setup({}); l.jsonls.setup({}); l.yamlls.setup({})
 end },
 { "hrsh7th/nvim-cmp", dependencies = { "hrsh7th/cmp-nvim-lsp","hrsh7th/cmp-buffer","hrsh7th/cmp-path","L3MON4D3/LuaSnip","saadparwaiz1/cmp_luasnip" }, config = function()
   local c = require("cmp")
   c.setup({ snippet = { expand = function(a) require("luasnip").lsp_expand(a.body) end }, mapping = c.mapping.preset.insert({ ["<C-Space>"] = c.mapping.complete(), ["<CR>"] = c.mapping.confirm({ select = true }) }), sources = c.config.sources({ {name="nvim_lsp"},{name="luasnip"},{name="path"},{name="buffer"} }) })
 end },
 { "nvim-telescope/telescope.nvim", dependencies = { "nvim-lua/plenary.nvim" } },
 { "nvim-neo-tree/neo-tree.nvim", branch = "v3.x", dependencies = { "nvim-lua/plenary.nvim","MunifTanjim/nui.nvim","nvim-tree/nvim-web-devicons" } },
 { "folke/which-key.nvim", event = "VeryLazy", opts = {} },
 { "lewis6991/gitsigns.nvim", opts = {} },
 { "nvim-lualine/lualine.nvim", dependencies = { "nvim-tree/nvim-web-devicons" }, opts = { options = { globalstatus = true } } },
 { "numToStr/Comment.nvim", opts = {} },
 { "windwp/nvim-autopairs", event = "InsertEnter", opts = {} },
 { "stevearc/conform.nvim", opts = { formatters_by_ft = { lua={"stylua"}, python={"black"}, javascript={"prettier"}, typescript={"prettier"}, json={"prettier"}, yaml={"prettier"} }, format_on_save = { timeout_ms = 1500, lsp_fallback = true } } },
 { "folke/trouble.nvim", dependencies = { "nvim-tree/nvim-web-devicons" }, opts = {} },
 { "catppuccin/nvim", name = "catppuccin", priority = 1000, config = function() vim.cmd.colorscheme("catppuccin-mocha") end },
}
