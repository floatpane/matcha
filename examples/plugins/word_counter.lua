-- word_counter.lua
-- Shows a live word count in the composer help bar.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    local words = 0
    for _ in state.body:gmatch("%S+") do
        words = words + 1
    end
    matcha.set_status("composer", words .. " words")
end)
