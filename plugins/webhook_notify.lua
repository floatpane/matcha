-- webhook_notify.lua
-- Posts a JSON payload to a webhook URL when an email is received.

local matcha = require("matcha")

local WEBHOOK_URL = "https://example.com/webhook"

matcha.on("email_received", function(email)
    local payload = '{"from":"' .. email.from .. '","subject":"' .. email.subject .. '"}'

    local res, err = matcha.http({
        url = WEBHOOK_URL,
        method = "POST",
        headers = { ["Content-Type"] = "application/json" },
        body = payload,
    })

    if err then
        matcha.log("webhook error: " .. err)
        return
    end

    if res.status >= 400 then
        matcha.log("webhook returned status " .. res.status)
    end
end)
