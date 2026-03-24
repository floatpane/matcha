-- keyword_highlighter.lua
-- Notifies you when incoming emails contain specific keywords.
-- Useful for tracking topics across your inbox.

local matcha = require("matcha")

local keywords = {
    "urgent",
    "deadline",
    "invoice",
    "payment",
    "meeting",
    "review",
}

matcha.on("email_received", function(email)
    local subj = email.subject:lower()
    for _, keyword in ipairs(keywords) do
        if subj:find(keyword, 1, true) then
            matcha.notify("[" .. keyword .. "] " .. email.subject, 3)
            return
        end
    end
end)
