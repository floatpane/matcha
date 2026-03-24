-- domain_filter.lua
-- Highlights emails from specific domains with a notification.
-- Useful for filtering work vs. personal emails.

local matcha = require("matcha")

local watched_domains = {
    "company.com",
    "work.org",
}

matcha.on("email_received", function(email)
    for _, domain in ipairs(watched_domains) do
        if email.from:match("@" .. domain:gsub("%.", "%%.") .. "$") then
            matcha.notify("Work email: " .. email.subject, 2)
            return
        end
    end
end)
