-- unread_counter.lua
-- Displays unread count in the inbox title bar.

local matcha = require("matcha")

local unread = 0

matcha.on("email_received", function(email)
    if not email.is_read then
        unread = unread + 1
    end
    matcha.set_status("inbox", unread .. " unread")
end)

matcha.on("folder_changed", function(folder)
    unread = 0
    matcha.set_status("inbox", "")
end)
