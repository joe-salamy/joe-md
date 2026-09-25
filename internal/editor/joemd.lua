-- joemd: reports the cursor line back to joe-md. Installed by joe-md; it does
-- nothing unless micro was launched by joe-md (JOE_MD_CURSOR_FILE is set).
VERSION = "1.0.0"

local micro = import("micro")
local goos = import("os")

local out = goos.Getenv("JOE_MD_CURSOR_FILE")
local target = goos.Getenv("JOE_MD_FILE")
local last = -1

-- onAnyEvent runs after every key, mouse and resize event, so the file always
-- holds the latest cursor line even if micro exits without a quit action.
function onAnyEvent()
    if out == "" then return end
    local bp = micro.CurPane()
    if bp == nil or bp.Buf == nil or bp.Buf.AbsPath ~= target then return end
    local y = bp.Cursor.Y
    if y == last then return end
    local f = io.open(out, "w")
    if f == nil then return end
    f:write(tostring(y + 1) .. "\n")
    f:close()
    last = y
end
