-- notify_github.lua
-- Shows a notification when emails from GitHub arrive.

local matcha = require("matcha")

matcha.on("email_received", function(email)
    if email.from:match("github%.com") then
        matcha.notify("GitHub: " .. email.subject, 3)
    end
end)
