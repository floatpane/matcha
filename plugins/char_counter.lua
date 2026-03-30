-- char_counter.lua
-- Shows a live character count in the composer help bar.

local matcha = require("matcha")

matcha.on("composer_updated", function(state)
    matcha.set_status("composer", state.body_len .. " chars")
end)
