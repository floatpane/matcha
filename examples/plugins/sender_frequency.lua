-- sender_frequency.lua
-- Tracks how many emails each sender sends you and shows repeat senders.

local matcha = require("matcha")

local senders = {}

matcha.on("email_received", function(email)
    local from = email.from
    senders[from] = (senders[from] or 0) + 1
    if senders[from] == 3 then
        matcha.notify("Frequent sender: " .. from .. " (3+ emails)", 2)
    end
end)

matcha.on("shutdown", function()
    local sorted = {}
    for sender, count in pairs(senders) do
        if count > 1 then
            table.insert(sorted, { addr = sender, count = count })
        end
    end
    table.sort(sorted, function(a, b) return a.count > b.count end)

    for i = 1, math.min(5, #sorted) do
        matcha.log("Top sender: " .. sorted[i].addr .. " (" .. sorted[i].count .. " emails)")
    end
end)
