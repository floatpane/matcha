-- reading_time.lua
-- Estimates reading time based on word count while composing.
-- Assumes ~200 words per minute average reading speed.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    local words = 0
    for _ in state.body:gmatch("%S+") do
        words = words + 1
    end
    if words > 0 then
        local minutes = math.ceil(words / 200)
        if minutes <= 1 then
            matcha.set_status("composer", "~1 min read")
        else
            matcha.set_status("composer", "~" .. minutes .. " min read")
        end
    end
end)
