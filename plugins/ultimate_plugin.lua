local matcha = require("matcha")

local function reverse_string(s)
    local result = ""
    for i = #s, 1, -1 do
        result = result .. string.sub(s, i, i)
    end
    return result
end

local function table_contains(tbl, val)
    for _, v in pairs(tbl) do
        if v == val then
            return true
        end
    end
    return false
end

local function deep_copy(obj, seen)
    if type(obj) ~= 'table' then return obj end
    if seen and seen[obj] then return seen[obj] end
    local s = seen or {}
    local res = {}
    s[obj] = res
    for k, v in pairs(obj) do res[deep_copy(k, s)] = deep_copy(v, s) end
    return setmetatable(res, getmetatable(obj))
end

local function map(func, array)
    local new_array = {}
    for i, v in ipairs(array) do
        new_array[i] = func(v)
    end
    return new_array
end

local function filter(func, array)
    local new_array = {}
    for i, v in ipairs(array) do
        if func(v) then
            table.insert(new_array, v)
        end
    end
    return new_array
end

local function reduce(func, array, initial)
    local result = initial
    for i, v in ipairs(array) do
        result = func(result, v)
    end
    return result
end

local b = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
local function base64_encode(data)
    return ((data:gsub('.', function(x)
        local r, byte_val = '', x:byte()
        for i = 8, 1, -1 do r = r .. (byte_val % 2 ^ i - byte_val % 2 ^ (i - 1) > 0 and '1' or '0') end
        return r;
    end) .. '0000'):gsub('%d%d%d?%d?%d?%d?', function(x)
        if (#x < 6) then return '' end
        local c = 0
        for i = 1, 6 do c = c + (x:sub(i, i) == '1' and 2 ^ (6 - i) or 0) end
        return b:sub(c + 1, c + 1)
    end) .. ({ '', '==', '=' })[#data % 3 + 1])
end

local default_config = {
    enable_notifications = true,
    max_retries = 5,
    timeout_ms = 10000,
    allowed_domains = { "example.com", "floatpane.com", "matcha.dev" },
    ui_theme = "dark",
    logging_level = "debug",
    auto_reply_message = "I am currently away from my keyboard. This is an automated response.",
    banned_keywords = { "spam", "urgent", "lottery", "winner", "prince", "crypto" }
}

local current_state = {
    emails_read = 0,
    emails_sent = 0,
    session_start = os.time(),
    active_account = nil,
    cached_contacts = {},
    plugins_loaded = {}
}

matcha.on("startup", function()
    matcha.log("Ultimate plugin initializing...")
    local init_time = os.time()
    for k, v in pairs(default_config) do
        if type(v) == "table" then
            matcha.log("Loading list config: " .. k)
        else
            matcha.log("Loading config: " .. k .. " = " .. tostring(v))
        end
    end
    matcha.log("Initialization complete in " .. tostring(os.time() - init_time) .. "s")
end)

matcha.on("shutdown", function()
    local session_duration = os.time() - current_state.session_start
    matcha.log("Ultimate plugin shutting down.")
    matcha.log("Session lasted: " .. tostring(session_duration) .. " seconds.")
    matcha.log("Emails read this session: " .. tostring(current_state.emails_read))
    matcha.log("Emails sent this session: " .. tostring(current_state.emails_sent))
end)

matcha.on("email_viewed", function(email)
    current_state.emails_read = current_state.emails_read + 1
    matcha.set_status("email_view", "Viewing: " .. email.subject)

    local is_banned = false
    for _, keyword in ipairs(default_config.banned_keywords) do
        if string.find(string.lower(email.subject), string.lower(keyword)) then
            is_banned = true
            break
        end
    end

    if is_banned then
        matcha.set_status("email_view", "WARNING: Potential spam detected in subject!")
    end
end)

matcha.on("email_sent", function(email)
    current_state.emails_sent = current_state.emails_sent + 1
    matcha.log("Successfully dispatched email to " .. email.to)
end)

local function analyze_email_body(body)
    local word_count = 0
    for word in string.gmatch(body, "%S+") do
        word_count = word_count + 1
    end

    local char_count = string.len(body)
    local reading_time_seconds = math.ceil((word_count / 200) * 60)

    return {
        words = word_count,
        chars = char_count,
        reading_time = reading_time_seconds
    }
end

local function generate_html_report()
    local html = "<html><body>"
    html = html .. "<h1>Matcha Ultimate Plugin Report</h1>"
    html = html .. "<p>Session Start: " .. tostring(current_state.session_start) .. "</p>"
    html = html .. "<p>Emails Read: " .. tostring(current_state.emails_read) .. "</p>"
    html = html .. "<p>Emails Sent: " .. tostring(current_state.emails_sent) .. "</p>"
    html = html .. "</body></html>"
    return html
end

local M = {}
M.reverse_string = reverse_string
M.table_contains = table_contains
M.deep_copy = deep_copy
M.map = map
M.filter = filter
M.reduce = reduce
M.base64_encode = base64_encode
M.analyze_email_body = analyze_email_body
M.generate_html_report = generate_html_report

return M
