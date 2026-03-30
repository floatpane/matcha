-- folder_announcer.lua
-- Shows a brief notification when you switch folders.

local matcha = require("matcha")

matcha.on("folder_changed", function(folder)
    matcha.notify("Switched to " .. folder, 1)
end)
