-- recipient_counter.lua
-- Shows the number of recipients in the composer status bar.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    if state.to == "" then
        return
    end
    local count = 0
    for _ in state.to:gmatch("[^,]+") do
        count = count + 1
    end
    if count > 1 then
        matcha.set_status("composer", count .. " recipients")
    end
end)
