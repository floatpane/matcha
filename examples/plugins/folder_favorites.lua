-- folder_favorites.lua
-- Tracks your most visited folders and logs the top 3 on shutdown.

local matcha = require("matcha")

local visits = {}

matcha.on("folder_changed", function(folder)
    visits[folder] = (visits[folder] or 0) + 1
end)

matcha.on("shutdown", function()
    -- Sort folders by visit count.
    local sorted = {}
    for folder, count in pairs(visits) do
        table.insert(sorted, { name = folder, count = count })
    end
    table.sort(sorted, function(a, b) return a.count > b.count end)

    local top = {}
    for i = 1, math.min(3, #sorted) do
        table.insert(top, sorted[i].name .. "(" .. sorted[i].count .. ")")
    end
    if #top > 0 then
        matcha.log("Top folders: " .. table.concat(top, ", "))
    end
end)
