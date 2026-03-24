-- inbox_activity.lua
-- Shows a live activity indicator in the inbox status bar.
-- Combines received, read, and sent counts into a compact display.

local matcha = require("matcha")

local received = 0
local sent = 0

local function update_status()
    matcha.set_status("inbox", "↓" .. received .. " ↑" .. sent)
end

matcha.on("email_received", function(email)
    received = received + 1
    update_status()
end)

matcha.on("email_send_after", function()
    sent = sent + 1
    update_status()
end)

matcha.on("startup", function()
    update_status()
end)
