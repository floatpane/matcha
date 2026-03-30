-- spam_detector.lua
-- Flags incoming emails that match common spam patterns.

local matcha = require("matcha")

local spam_patterns = {
    "you have won",
    "claim your prize",
    "act now",
    "limited time offer",
    "click here immediately",
    "congratulations!",
    "urgent action required",
    "unsubscribe",
}

matcha.on("email_received", function(email)
    local subj = email.subject:lower()
    for _, pattern in ipairs(spam_patterns) do
        if subj:find(pattern, 1, true) then
            matcha.notify("Possible spam: " .. email.subject, 3)
            return
        end
    end
end)
