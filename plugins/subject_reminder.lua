-- subject_reminder.lua
-- Warns you if you're composing an email without a subject line.

local matcha = require("matcha")

matcha.on("email_send_before", function(email)
    if email.subject == "" then
        matcha.notify("Warning: no subject line!", 3)
    end
end)
