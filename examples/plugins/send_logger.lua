-- send_logger.lua
-- Logs every email you send for personal record-keeping.

local matcha = require("matcha")

matcha.on("email_send_before", function(email)
    matcha.log("Sending to: " .. email.to .. " | Subject: " .. email.subject)
end)

matcha.on("email_send_after", function()
    matcha.notify("Email sent successfully!", 3)
end)
