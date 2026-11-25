-- joemd: reports the cursor line back to joe-md. Installed by joe-md; it does
-- nothing unless micro was launched by joe-md (JOE_MD_CURSOR_FILE is set).
--
-- The line is written before micro quits, from a pre hook: micro exits from
-- inside its Quit action (ForceQuit calls runtime.Goexit), so hooks that run
-- after an action, such as onQuit, never run when micro closes.
VERSION = "1.1.0"

local micro = import("micro")
local goos = import("os")

local out = goos.Getenv("JOE_MD_CURSOR_FILE")
local target = goos.Getenv("JOE_MD_FILE")
local last = -1

-- cursorLine is the 1-based line of the cursor in the pane showing target, or
-- nil if another pane (or no pane) is current. CurPane and the fields it leads
-- to panic on panes that are not text buffers, hence the pcall.
local function cursorLine()
    local ok, line = pcall(function()
        local bp = micro.CurPane()
        if bp == nil or bp.Buf == nil or bp.Buf.AbsPath ~= target then return nil end
        return bp.Cursor.Y + 1
    end)
    if not ok then return nil end
    return line
end

local function write()
    if out == "" then return end
    local line = cursorLine()
    if line == nil or line == last then return end
    local f = io.open(out, "w")
    if f == nil then return end
    f:write(tostring(line) .. "\n")
    f:close()
    last = line
end

-- preQuit runs before micro quits, and before it asks to save; every way of
-- quitting micro's current buffer starts from one of these. preQuit and
-- friends exist in all micro versions.
function preQuit() write() end
function preForceQuit() write() end
function preQuitAll() write() end

-- micro 2.0.14 and later call onAnyEvent for every key, mouse and resize
-- event, so the file also holds the latest line if micro dies without
-- quitting. Older versions never call it, which is what the pre hooks cover.
function onAnyEvent() write() end
