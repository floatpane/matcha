-- subject_length_warn.lua
-- Warns when your subject line is getting too long.
-- Most email clients truncate subjects beyond ~60 characters.
--
-- Thresholds are configurable in Settings → Plugins.

local matcha = require("matcha")

local cfg = matcha.settings({
    enabled = {
        type = "boolean",
        default = true,
        label = "Enable subject length warnings",
    },
    soft_limit = {
        type = "number",
        default = 60,
        label = "Soft limit (chars)",
        description = "Warn that the subject may truncate above this length.",
    },
    hard_limit = {
        type = "number",
        default = 78,
        label = "Hard limit (chars)",
        description = "Warn that the subject is too long above this length.",
    },
})

matcha.on("composer_updated", function(state)
    if not cfg.enabled then
        return
    end
    local len = #state.subject
    if len > cfg.hard_limit then
        matcha.set_status("composer", "Subject too long (" .. len .. " chars)")
    elseif len > cfg.soft_limit then
        matcha.set_status("composer", "Subject may truncate (" .. len .. " chars)")
    end
end)
