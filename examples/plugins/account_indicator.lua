-- account_indicator.lua
-- Shows which account received the email you're currently viewing.

local matcha = require("matcha")

matcha.on("email_viewed", function(email)
    matcha.set_status("email_view", "Account: " .. email.account_id)
end)
