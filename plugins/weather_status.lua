-- weather_status.lua
-- Fetches current weather and displays it in the inbox status bar.
-- Uses the free wttr.in API (no API key required).

local matcha = require("matcha")

local CITY = "London"

matcha.on("startup", function()
    local res, err = matcha.http({
        url = "https://wttr.in/" .. CITY .. "?format=%t+%C",
    })

    if err then
        matcha.log("weather: " .. err)
        return
    end

    if res.status == 200 then
        matcha.set_status("inbox", res.body)
    end
end)
