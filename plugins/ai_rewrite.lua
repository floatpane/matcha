-- ai_rewrite.lua
-- Rewrites the email body using an AI model.
-- Press ctrl+r in the composer to open the prompt overlay.
--
-- Configuration: Set the API_URL, API_KEY, and MODEL variables below.
-- Works with any OpenAI-compatible API (OpenAI, Ollama, llama.cpp, etc).

local matcha        = require("matcha")

-- Configuration
local API_URL       = "http://localhost:11434/v1/chat/completions" -- Ollama default
local API_KEY       = ""                                           -- not needed for Ollama
local MODEL         = "llama3"

local SYSTEM_PROMPT = [[You are an email rewriting assistant inside an email client.

You will receive an email body that the user has already written, along with an instruction on how to rewrite it. Your job is to rephrase the existing text according to the instruction.

Critical rules:
- Output ONLY the rewritten email body. Nothing else.
- Do NOT include a subject line.
- Do NOT include a signature, sign-off, closing, or sender name (e.g. "Best regards, X", "Sincerely, X", "Thanks, X"). The email client appends the signature automatically.
- Do NOT include the sender's name or email address anywhere in the output.
- Do NOT wrap the output in quotes, markdown code blocks, or any formatting markers.
- Do NOT add any preamble like "Here is the rewritten email:" or "Your email:".
- Do NOT invent, assume, or add any facts, details, or information not present in the original email body. Only rephrase what is already there.
- If the recipient's name is needed for a greeting, extract it from the To address. Use only the first name. If no display name is available, omit the name from the greeting entirely (just use "Hi," or "Hello,").
- Preserve the original intent, meaning, and all key information.
- Match the tone and style requested by the user.
- Keep similar length unless the user asks to shorten or expand.]]

matcha.bind_key("ctrl+r", "composer", "ai rewrite", function(state)
    matcha.prompt("Rewrite instruction (e.g. 'make it more formal'):", function(instruction)
        local body = state.body
        if body == "" then
            matcha.notify("Nothing to rewrite", 2)
            return
        end

        local user_msg = string.format(
            "To: %s\nSubject: %s\nInstruction: %s\n\nEmail body:\n%s",
            state.to, state.subject, instruction, body
        )

        local payload = string.format(
            '{"model":"%s","messages":[{"role":"system","content":"%s"},{"role":"user","content":"%s"}]}',
            MODEL,
            SYSTEM_PROMPT:gsub('"', '\\"'):gsub('\n', '\\n'),
            user_msg:gsub('"', '\\"'):gsub('\n', '\\n')
        )

        local headers = { ["Content-Type"] = "application/json" }
        if API_KEY ~= "" then
            headers["Authorization"] = "Bearer " .. API_KEY
        end

        matcha.notify("Rewriting...", 10)

        local res, err = matcha.http({
            url = API_URL,
            method = "POST",
            headers = headers,
            body = payload,
        })

        if err then
            matcha.notify("AI error: " .. err, 3)
            return
        end

        if res.status ~= 200 then
            matcha.notify("AI returned status " .. res.status, 3)
            return
        end

        -- Extract content from OpenAI-compatible response.
        -- Response format: {"choices":[{"message":{"content":"..."}}]}
        local content = res.body:match('"content"%s*:%s*"(.-)"')
        if not content then
            matcha.notify("Could not parse AI response", 3)
            return
        end

        -- Unescape JSON string
        content = content:gsub('\\n', '\n')
        content = content:gsub('\\"', '"')
        content = content:gsub('\\\\', '\\')

        matcha.set_compose_field("body", content)
        matcha.notify("Email rewritten", 2)
    end)
end)
