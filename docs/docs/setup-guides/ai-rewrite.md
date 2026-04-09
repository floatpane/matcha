---
title: AI Rewrite Plugin
sidebar_position: 3
---

# Setting up the AI Rewrite Plugin

Matcha includes an `ai_rewrite.lua` plugin that allows you to rewrite email drafts using an AI model. By default, it works with any OpenAI-compatible API.

To get started, install the plugin:

```bash
matcha install https://raw.githubusercontent.com/floatpane/matcha/master/plugins/ai_rewrite.lua
```

You can then edit `~/.config/matcha/plugins/ai_rewrite.lua` to configure it for your preferred AI provider (`matcha config ai_rewrite`). Since the plugin relies on the OpenAI chat completions format, it seamlessly integrates with OpenAI, local providers like Ollama, and other services that offer an OpenAI-compatible endpoint (like Gemini). For providers without native OpenAI compatibility (like Claude), you can use a local proxy like [LiteLLM](https://github.com/BerriAI/litellm).

Here are the configuration snippets for various popular AI providers. Update the variables at the top of your `ai_rewrite.lua` file.

---

## Ollama (Local)

Ollama provides a local, private OpenAI-compatible endpoint out of the box. Make sure you have pulled a model first (e.g., `ollama run llama3`).

```lua
-- Configuration for Ollama
local API_URL       = "http://localhost:11434/v1/chat/completions"
local API_KEY       = "" -- Not needed for Ollama
local MODEL         = "llama3" -- Replace with your downloaded model name
```

---

## OpenAI

To use OpenAI's models, you will need an API key from your [OpenAI platform dashboard](https://platform.openai.com/api-keys).

```lua
-- Configuration for OpenAI
local API_URL       = "https://api.openai.com/v1/chat/completions"
local API_KEY       = "sk-proj-YOUR_OPENAI_API_KEY"
local MODEL         = "gpt-4o-mini" -- Or "gpt-4o", "gpt-3.5-turbo", etc.
```

---

## Google Gemini

Google Gemini recently introduced an OpenAI-compatible endpoint. You can use your [Gemini API Key](https://aistudio.google.com/app/apikey) directly.

```lua
-- Configuration for Google Gemini
local API_URL       = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
local API_KEY       = "YOUR_GEMINI_API_KEY"
local MODEL         = "gemini-1.5-flash" -- Or "gemini-1.5-pro"
```

---

## Anthropic Claude

Anthropic's API does not natively use the OpenAI format. However, you can easily use Claude by running a lightweight local proxy like **LiteLLM**, which translates OpenAI-formatted requests into Anthropic's format.

1. Install and start LiteLLM with your Anthropic key:
   ```bash
   pip install litellm
   litellm --model claude-3-5-sonnet-20241022 --api_key YOUR_ANTHROPIC_API_KEY
   ```
2. LiteLLM will start a local server, usually on port `4000`.

Configure the plugin to point to your LiteLLM instance:

```lua
-- Configuration for Claude (via LiteLLM proxy)
local API_URL       = "http://localhost:4000/v1/chat/completions"
local API_KEY       = "" -- Handled by LiteLLM
local MODEL         = "claude-3-5-sonnet-20241022" -- Must match the LiteLLM model
```

---

## Usage

Once configured:
1. Restart Matcha to load the plugin.
2. Open the composer.
3. Draft your email.
4. Press `ctrl+r` to trigger the AI Rewrite prompt.
5. Provide an instruction (e.g., *"Make it more formal"*, *"Fix typos"*, *"Shorten it"*).
6. The AI will rewrite your draft and replace the body content automatically.
