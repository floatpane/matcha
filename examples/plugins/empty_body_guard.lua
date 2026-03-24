-- empty_body_guard.lua
-- Warns before sending an email with an empty body.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    if state.body_len == 0 and state.to ~= "" then
        matcha.set_status("composer", "Email body is empty")
    end
end)
