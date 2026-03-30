-- email_age.lua
-- Shows the date of the email you're viewing in the status bar.

local matcha = require("matcha")

matcha.on("email_viewed", function(email)
    -- Show the date portion of the RFC3339 timestamp.
    local date = email.date:match("^(%d%d%d%d%-%d%d%-%d%d)")
    if date then
        matcha.set_status("email_view", "Date: " .. date)
    end
end)
