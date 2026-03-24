-- attachment_reminder.lua
-- Warns if your email body mentions an attachment but you might have
-- forgotten to attach it. Checks common phrases before sending.

local matcha = require("matcha")

local phrases = {
    "attach",
    "attached",
    "attachment",
    "enclosed",
    "find attached",
    "see attached",
    "i've attached",
}

matcha.on("composer_updated", function(state)
    local body = state.body:lower()
    for _, phrase in ipairs(phrases) do
        if body:find(phrase, 1, true) then
            matcha.set_status("composer", "Don't forget the attachment!")
            return
        end
    end
end)
