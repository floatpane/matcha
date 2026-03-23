-- hello.lua
-- A minimal example plugin that logs lifecycle events.

local matcha = require("matcha")

matcha.on("startup", function()
  matcha.log("hello plugin loaded")
end)

matcha.on("shutdown", function()
  matcha.log("hello plugin shutting down, goodbye!")
end)
