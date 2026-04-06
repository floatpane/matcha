---
title: AI Agents
sidebar_position: 11
---

# AI Agent Integration

Matcha can be used by AI coding agents (like Claude Code) to send emails on your behalf. This is powered by the [`matcha send`](./CLI.md) CLI command and an optional Claude Code skill.

## How It Works

The `matcha send` command provides a non-interactive interface for sending emails. AI agents can construct and execute this command to send emails without launching the TUI.

```bash
matcha send --to alice@example.com --subject "Meeting notes" --body "Here are the notes from today."
```

The agent can:

- Send to one or multiple recipients (`--to`, `--cc`, `--bcc`)
- Choose which account to send from (`--from`)
- Attach files (`--attach`)
- Pipe in long message bodies from stdin (`--body -`)
- Control signing and encryption (`--sign-smime`, `--sign-pgp`, `--encrypt-smime`)

See the full [CLI reference](./CLI.md) for all available flags.

Matcha ships with a built-in skill at `SKILLS/send-email/`.

### What the Skill Does

When you ask an AI agent to send an email, the skill provides it with:

- The correct `matcha send` command syntax
- All available flags and their usage
- Instructions to confirm with you before sending
- Examples for common scenarios

### Usage

Just ask the agent naturally:

> "Send an email to alice@example.com about the project deadline"

> "Email the team the weekly report from my work account"

> "Send bob the meeting notes with the slides attached"

The agent will compose the appropriate `matcha send` command, show it to you for confirmation, and execute it.

### Installing the Skill Globally for Claude Code

The skill is included in the matcha repository at `./SKILLS/send-email/SKILL.md`. To make it available in all your projects, symlink it to your global Claude Code skills directory:

```bash
mkdir -p ~/.claude/skills
ln -s /path/to/matcha/.claude/skills/send-email ~/.claude/skills/send-email
```

After linking, the `send-email` skill will be available in any project where you use Claude Code.

## Scripting and Automation

Beyond AI agents, `matcha send` works in any automation context:

**Cron job for daily reports:**

```bash
# Send a daily digest at 9am
0 9 * * * cat /tmp/daily-report.md | matcha send --to team@company.com --subject "Daily Report" --body -
```

**Git hook for notifications:**

```bash
#!/bin/sh
matcha send --to reviewer@company.com --subject "New commit pushed" \
  --body "A new commit was pushed to the repository."
```

**CI/CD pipeline notifications:**

```bash
matcha send --to devops@company.com --subject "Deploy complete" \
  --body "Version $VERSION deployed to production." --signature=false
```

## Prerequisites

- At least one email account must be configured in matcha. Run `matcha` (the TUI) to set up an account if you haven't already.
- The account's password or OAuth2 tokens must be stored in the system keyring.
