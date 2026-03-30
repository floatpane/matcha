-- read_tracker.lua
-- Displays a running count of emails you've read this session.

local matcha = require("matcha")

local read_count = 0

matcha.on("email_viewed", function(email)
    read_count = read_count + 1
    matcha.set_status("email_view", read_count .. " read this session")
end)
