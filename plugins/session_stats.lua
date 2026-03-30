-- session_stats.lua
-- Tracks emails received, read, and sent during your session.

local matcha = require("matcha")

local stats = { received = 0, read = 0, sent = 0 }

matcha.on("email_received", function(email)
    stats.received = stats.received + 1
end)

matcha.on("email_viewed", function(email)
    stats.read = stats.read + 1
end)

matcha.on("email_send_after", function()
    stats.sent = stats.sent + 1
end)

matcha.on("shutdown", function()
    matcha.log("Session stats: "
        .. stats.received .. " received, "
        .. stats.read .. " read, "
        .. stats.sent .. " sent")
end)
