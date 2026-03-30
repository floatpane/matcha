-- vip_alerts.lua
-- Shows prominent notifications for emails from important senders.
-- Add your VIP addresses to the list below.

local matcha = require("matcha")

local vips = {
    "boss@example.com",
    "ceo@example.com",
    "partner@example.com",
}

matcha.on("email_received", function(email)
    for _, vip in ipairs(vips) do
        if email.from:find(vip, 1, true) then
            matcha.notify("VIP: " .. email.from .. " - " .. email.subject, 5)
            return
        end
    end
end)
