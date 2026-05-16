-- prevent_auto_read.lua
-- Prevents emails from being automatically marked as read when opened.
-- When enabled, emails stay unread until explicitly marked (e.g. via toggle_read).

local matcha = require("matcha")

local cfg = matcha.settings({
    enabled = {
        type        = "boolean",
        default     = true,
        label       = "Prevent auto-read",
        description = "When on, opening an email does not mark it as read.",
    },
})

matcha.on("email_viewed", function(email)
    if cfg.enabled then
        matcha.suppress_auto_read()
    end
end)
