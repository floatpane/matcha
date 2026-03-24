-- greeting.lua
-- Shows a random motivational greeting on startup.

local matcha = require("matcha")

local greetings = {
    "Ready to tackle your inbox!",
    "Let's get through those emails.",
    "Inbox zero awaits!",
    "You've got mail... probably.",
    "Time to reply to that email from last week.",
    "Email: the original social network.",
}

matcha.on("startup", function()
    local pick = greetings[math.random(#greetings)]
    matcha.notify(pick, 3)
end)
