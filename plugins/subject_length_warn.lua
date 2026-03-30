-- subject_length_warn.lua
-- Warns when your subject line is getting too long.
-- Most email clients truncate subjects beyond ~60 characters.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    local len = #state.subject
    if len > 78 then
        matcha.set_status("composer", "Subject too long (" .. len .. " chars)")
    elseif len > 60 then
        matcha.set_status("composer", "Subject may truncate (" .. len .. " chars)")
    end
end)
