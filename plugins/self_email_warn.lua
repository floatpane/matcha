-- self_email_warn.lua
-- Notifies you when you receive an email you sent to yourself.

local matcha = require("matcha")

matcha.on("email_received", function(email)
    for i = 1, #email.to do
        if email.to[i] == email.from then
            matcha.notify("Self-email: " .. email.subject, 2)
            return
        end
    end
end)
