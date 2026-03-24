-- reply_all_warn.lua
-- Warns when sending to many recipients (possible accidental reply-all).

local matcha = require("matcha")

local threshold = 5

matcha.on("email_send_before", function(email)
    local count = 0
    for _ in email.to:gmatch("[^,]+") do
        count = count + 1
    end
    for _ in email.cc:gmatch("[^,]+") do
        count = count + 1
    end
    if count >= threshold then
        matcha.notify("Heads up: sending to " .. count .. " recipients", 3)
    end
end)
