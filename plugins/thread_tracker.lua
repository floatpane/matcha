-- thread_tracker.lua
-- Tracks how many replies and forwards you receive per session.

local matcha = require("matcha")

local replies = 0
local forwards = 0

matcha.on("email_received", function(email)
    local subj = email.subject:lower()
    if subj:match("^re:") or subj:match("^re%[%d+%]:") then
        replies = replies + 1
    elseif subj:match("^fwd:") or subj:match("^fw:") then
        forwards = forwards + 1
    end
    matcha.set_status("inbox", replies .. " replies, " .. forwards .. " fwd")
end)
