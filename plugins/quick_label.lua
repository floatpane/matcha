-- quick_label.lua
-- Demonstrates custom keyboard shortcuts with matcha.bind_key().
-- Press ctrl+i in the inbox to show the selected email's subject.

local matcha = require("matcha")

matcha.bind_key("ctrl+i", "inbox", "info", function(email)
    if email then
        matcha.notify(email.from .. ": " .. email.subject, 3)
    end
end)
