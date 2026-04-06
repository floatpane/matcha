-- auto_bcc.lua
-- Automatically adds a BCC address to every email you compose.
-- Change the address below to your own archive/backup address.

local matcha = require("matcha")

local bcc_address = "archive@example.com"

matcha.on("composer_updated", function(state)
    if state.bcc == "" then
        matcha.set_compose_field("bcc", bcc_address)
    end
end)
