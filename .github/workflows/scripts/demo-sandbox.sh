#!/usr/bin/env bash
# Builds a throwaway $HOME with a Maildir account full of fake mail for
# demo.tape, so the README GIF never touches a real inbox.
# Usage: .github/workflows/scripts/demo-sandbox.sh [sandbox_dir]
# (default sandbox_dir: <repo>/.demo-sandbox, which demo.tape expects)
# Set MATCHA_BIN to use a prebuilt binary instead of building this checkout.
# Works on macOS and Linux (needs python3 for portable date handling).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SANDBOX="${1:-$ROOT/.demo-sandbox}"
rm -rf "$SANDBOX"

# Build matcha from this checkout and put a quiet wrapper on the sandbox PATH
# (stderr logging would otherwise bleed into the recorded TUI).
mkdir -p "$SANDBOX/bin"
if [[ -n "${MATCHA_BIN:-}" ]]; then
  cp "$MATCHA_BIN" "$SANDBOX/bin/matcha-bin"
else
  (cd "$ROOT" && go build -o "$SANDBOX/bin/matcha-bin" . 2>/dev/null)
fi
cat > "$SANDBOX/bin/matcha" <<'SH'
#!/bin/sh
exec "$(dirname "$0")/matcha-bin" "$@" 2>/dev/null
SH
chmod +x "$SANDBOX/bin/matcha"

MAIL="$SANDBOX/Mail/demo"
mkdir -p "$SANDBOX/.config/matcha" "$SANDBOX/.cache/matcha"
for d in "" .Sent .Archive; do mkdir -p "$MAIL/$d/cur" "$MAIL/$d/new" "$MAIL/$d/tmp"; done

cat > "$SANDBOX/.config/matcha/config.json" <<JSON
{
  "accounts": [
    {
      "id": "demo",
      "name": "Ada Lovelace",
      "email": "ada@matcha.email",
      "service_provider": "custom",
      "protocol": "maildir",
      "maildir_path": "$MAIL"
    }
  ],
  "disable_daemon": true,
  "disable_notifications": true,
  "hide_tips": true,
  "has_seen_setup_guide": true,
  "enable_split_pane": true,
  "theme": "Matcha"
}
JSON

# Skip the one-time matcha: URL handler registration; it talks to the
# system-wide LaunchServices database, which a sandbox must not touch.
touch "$SANDBOX/.config/matcha/.protocol_registered"

n=0
# msg <folder> <flags> <from> <subject> <minutes_ago> <body...>
msg() {
  local folder="$1" flags="$2" from="$3" subject="$4" ago="$5"; shift 5
  n=$((n + 1))
  local ts=$(( $(date +%s) - ago * 60 ))
  local date; date=$(python3 -c 'import sys, email.utils; print(email.utils.formatdate(int(sys.argv[1]), localtime=True))' "$ts")
  local dir="$MAIL/$folder/cur"
  local file="$dir/${ts}.demo${n}.matcha:2,${flags}"
  {
    printf 'From: %s\nTo: Ada Lovelace <ada@matcha.email>\nSubject: %s\nDate: %s\n' "$from" "$subject" "$date"
    printf 'Message-ID: <demo%d@matcha.email>\nMIME-Version: 1.0\nContent-Type: text/plain; charset=utf-8\n\n' "$n"
    printf '%s\n' "$@"
  } > "$file"
  python3 -c 'import os, sys; t = int(sys.argv[2]); os.utime(sys.argv[1], (t, t))' "$file" "$ts"
}

msg "" "" "Grace Hopper <grace@compiler.dev>" "Launch checklist for v1.0 🚀" 4 \
  "Hey Ada," "" \
  "We're nearly there. Before we tag v1.0 tomorrow:" "" \
  "  1. Final pass on the release notes" \
  "  2. Smoke-test the Homebrew + Nix installs" \
  "  3. Record the demo video (the fun part)" "" \
  "Ping me once the build is green and I'll hit publish." "" "— Grace"
msg "" "" "GitHub <noreply@github.com>" "[floatpane/matcha] PR #1720: Add threaded view" 18 \
  "linus-t requested your review on this pull request." "" \
  "  + Group conversations by Message-ID / References" \
  "  + Collapse and expand threads with T" "" \
  "View it on GitHub."
msg "" "S" "Alan Turing <alan@bletchley.org>" "Re: Lunch on Thursday?" 55 \
  "Thursday works. The usual place at 12:30?" "" "Alan"
msg "" "" "Linear <notifications@linear.app>" "MAT-212 moved to In Review" 90 \
  "Ada Lovelace moved MAT-212 \"Inline image previews\" to In Review."
msg "" "S" "Margaret Hamilton <margaret@apollo.space>" "Design review notes" 180 \
  "Thanks for the walkthrough today. Main takeaways:" "" \
  "  - Keyboard-first everything" \
  "  - Zero mouse required, but nice when it's there" \
  "  - Ship themes, people love themes" "" "M."
msg "" "S" "Stripe <receipts@stripe.com>" "Your receipt from Floatpane #2041-1188" 300 \
  "Amount paid: \$12.00" "Thanks for supporting open source."
msg "" "S" "Dennis Ritchie <dmr@bell-labs.com>" "Re: K&R second edition" 600 \
  "Happy to send you a signed copy. Still the best way to learn C."
msg "" "S" "Hacker Newsletter <hi@hackernewsletter.com>" "#712: terminals are cool again" 1500 \
  "This week: TUIs, Bubble Tea, and why your inbox belongs in a terminal."
msg "" "S" "Ken Thompson <ken@plan9.org>" "UTF-8, napkin edition" 2900 \
  "Sketched it on a placemat at dinner. Scan attached in spirit."
msg "" "S" "Barbara Liskov <barbara@mit.edu>" "Substitution principle talk" 4400 \
  "Slides are up. Let me know if you want the speaker notes too."
msg ".Sent" "S" "Ada Lovelace <ada@matcha.email>" "Re: Design review notes" 170 \
  "Agreed on all three. Themes are already in."
msg ".Archive" "S" "Charles Babbage <charles@engine.co>" "Difference Engine invoice" 9000 \
  "Attached is the final invoice for the analytical bits."

echo "$SANDBOX"
