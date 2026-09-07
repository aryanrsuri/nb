local M = {}
local config = { executable = 'nb' }

local function execute(argv, quiet)
  local ok, result = pcall(function() return vim.system(argv, { text = true }):wait() end)
  if not ok or result.code ~= 0 then
    if not quiet then
      vim.notify(ok and vim.trim(result.stderr) or 'nb: ' .. tostring(result), vim.log.levels.ERROR)
    end
    return nil
  end
  local value = vim.json.decode(result.stdout)
  if value ~= vim.NIL then return value end
end

local function run(root, command, ...)
  local argv = { config.executable, '-root', root, '-json', command }
  vim.list_extend(argv, { ... })
  return execute(argv)
end

local function context(require_note, quiet, buffer)
  buffer = buffer or vim.api.nvim_get_current_buf()
  local path = vim.api.nvim_buf_get_name(buffer)
  local note = path:sub(-3) == '.md' and path or nil
  if require_note and not note then
    if not quiet then vim.notify('nb: current buffer must be a Markdown note', vim.log.levels.WARN) end
    return nil
  end
  local start = note and vim.fs.dirname(note) or vim.fn.getcwd()
  local root = execute({ config.executable, '-root', start, '-json', 'root' }, quiet)
  if root then return { root = root, note = note, buffer = buffer } end
end

local function open(path, line, column)
  vim.api.nvim_cmd({ cmd = 'edit', args = { path } }, {})
  if line then
    line = math.min(line, vim.api.nvim_buf_line_count(0))
    local text = vim.api.nvim_buf_get_lines(0, line - 1, line, false)[1]
    vim.api.nvim_win_set_cursor(0, { line, math.min((column or 1) - 1, #text) })
  end
end

local function pick(title, entries, display, select)
  if not entries then return end
  local actions = require('telescope.actions')
  local state = require('telescope.actions.state')
  require('telescope.pickers').new({}, {
    prompt_title = title,
    finder = require('telescope.finders').new_table({
      results = entries,
      entry_maker = function(value)
        local label = display(value)
        return { value = value, display = label, ordinal = label }
      end,
    }),
    sorter = require('telescope.config').values.generic_sorter({}),
    attach_mappings = function(buffer)
      actions.select_default:replace(function()
        local entry = state.get_selected_entry()
        actions.close(buffer)
        if entry then select(entry.value) end
      end)
      return true
    end,
  }):find()
end

function M.find()
  local ctx = context(false)
  if not ctx then return end
  pick('Notes', run(ctx.root, 'list'), function(note) return note end, function(note)
    open(ctx.root .. '/' .. note .. '.md')
  end)
end

function M.follow()
  local ctx = context(true)
  if not ctx then return end
  local text = vim.api.nvim_get_current_line()
  local column = vim.api.nvim_win_get_cursor(0)[2] + 1
  local offset = 1
  while true do
    local first, last, target = text:find('%[%[([^%[%]]+)%]%]', offset)
    if not first then return end
    if column >= first and column <= last then
      local path = run(ctx.root, 'resolve', ctx.note, target)
      if path then open(path) end
      return
    end
    offset = last + 1
  end
end

local function links(kind)
  local ctx = context(kind ~= 'all')
  if not ctx then return end
  local entries
  if kind == 'all' then entries = run(ctx.root, 'links') else entries = run(ctx.root, kind, ctx.note) end
  pick(kind == 'all' and 'All links' or kind == 'backlinks' and 'Backlinks' or 'Outgoing links', entries,
    function(link) return link.source .. ':' .. link.line .. ' → ' .. link.target end,
    function(link)
      if kind == 'links' then
        open(ctx.root .. '/' .. link.target .. '.md')
      else
        open(ctx.root .. '/' .. link.source .. '.md', link.line, link.column)
      end
    end)
end

function M.links() links('links') end
function M.backlinks() links('backlinks') end
function M.all_links() links('all') end

function M.insert()
  local ctx = context(true)
  if not ctx then return end
  local cursor = vim.api.nvim_win_get_cursor(0)
  pick('Insert link', run(ctx.root, 'list'), function(target) return target end, function(target)
    local link = run(ctx.root, 'link', ctx.note, target)
    if not link or not vim.api.nvim_buf_is_valid(ctx.buffer) then return end
    vim.api.nvim_buf_set_text(ctx.buffer, cursor[1] - 1, cursor[2], cursor[1] - 1, cursor[2], { link })
    vim.api.nvim_set_current_buf(ctx.buffer)
    local column = cursor[2] + #link
    vim.api.nvim_win_set_cursor(0, { cursor[1], column })
  end)
end

function M.setup(opts)
  opts = opts or {}
  for key in pairs(opts) do
    if key ~= 'executable' then error('nb: unknown setup option ' .. key .. '; initialize notebooks with nb init') end
  end
  config = { executable = opts.executable or 'nb' }
  local bindings = {
    { 'gf', 'Follow', M.follow },
    { '<leader>nf', 'Find', M.find },
    { '<leader>nl', 'Links', M.links },
    { '<leader>nb', 'Backlinks', M.backlinks },
    { '<leader>na', 'AllLinks', M.all_links },
    { '<leader>ni', 'Insert', M.insert },
  }
  for _, binding in ipairs(bindings) do
    vim.api.nvim_create_user_command('Nb' .. binding[2], binding[3], { force = true })
  end
  local function attach(args)
    local buffer = args and args.buf or vim.api.nvim_get_current_buf()
    local ctx = context(true, true, buffer)
    for _, binding in ipairs(bindings) do
      local modes = { 'n' }
      if ctx then
        vim.keymap.set(modes, binding[1], binding[3], { buffer = buffer, desc = 'nb: ' .. binding[2] })
      else
        for _, mode in ipairs(modes) do
          for _, mapping in ipairs(vim.api.nvim_buf_get_keymap(buffer, mode)) do
            if mapping.callback == binding[3] then
              vim.keymap.del(mode, mapping.lhs, { buffer = buffer })
            end
          end
        end
      end
    end
  end
  vim.api.nvim_create_autocmd({ 'BufEnter', 'FileType', 'BufFilePost' }, {
    group = vim.api.nvim_create_augroup('nb', { clear = true }),
    callback = attach,
  })
  attach()
end

return M
