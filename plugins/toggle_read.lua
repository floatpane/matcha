-- toggle_read.lua
-- Toggle read/unread on the selected email. Keybind is configurable.

local matcha = require("matcha")

local cfg = matcha.settings({
    key = {
        type        = "string",
        default     = "u",
        label       = "Toggle key",
        description = "Key to press in inbox and email_view to toggle read/unread. Takes effect after restart.",
    },
})

local function toggle(email)
    if not email then return end
    local folder = email.folder ~= "" and email.folder or "INBOX"
    if email.is_read then
        matcha.mark_unread(email.uid, email.account_id, folder)
        matcha.notify("Marked as unread")
    else
        matcha.mark_read(email.uid, email.account_id, folder)
        matcha.notify("Marked as read")
    end
end

matcha.bind_key(cfg.key, "email_view", "Toggle read/unread", toggle)
matcha.bind_key(cfg.key, "inbox", "Toggle read/unread", toggle)
