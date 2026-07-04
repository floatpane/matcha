---
title: Patch Email Support
---

# Patch Email Support

Matcha has first-class support for the **git mail workflow** — sending,
receiving, viewing, and applying patches over email, without ever shelling out
to `git apply` or `git send-email` for the core operations.

This covers four user-facing flows:

| Flow | CLI | TUI key |
|------|-----|---------|
| Send a patch | `matcha send-patch` | `P` (from any email) |
| Apply a received patch | `matcha apply` | `p` (from a patch email) |
| View a patch with syntax-highlighted diff | — | automatic |
| Apply + commit in one step | — | `p` then confirm |

---

## Architecture

Two standalone libraries do the heavy lifting:

| Library | Role |
|---------|------|
| [`go-mailpatch`](https://github.com/floatpane/go-mailpatch) | Parses RFC 5322 `git format-patch` emails into structured diffs, series metadata, and commit messages |
| [`go-patchapply`](https://github.com/floatpane/go-patchapply) | Applies structured diffs to a directory filesystem (no git), generates patches via `git format-patch --stdout` |

The `gitmail` package (`gitmail/gitmail.go`) is the thin bridge that combines
both: `mailpatch.ParseBytes()` → `patchapply.ApplyPatch()`.

### Key source files

| File | Responsibility |
|------|----------------|
| `gitmail/gitmail.go` | Core bridge: `Apply`, `ApplySeries`, `GeneratePatch`, `ParsePatch`, `IsPatch` |
| `cli/gitmail_apply.go` | `matcha apply` CLI command |
| `cli/gitmail_send.go` | `matcha send-patch` CLI command |
| `view/patch.go` | Patch detection (`DetectPatch`) and TUI rendering (`RenderPatchBody`) |
| `view/diffview/diffview.go` | Full-width solid-block diff rendering with line numbers and syntax highlighting |
| `view/highlight.go` | Fallback diff highlighting (`HighlightDiff`) |
| `tui/email_view.go` | Email viewer — patch display, keybinding dispatch for `p` / `P` |
| `tui/patch_send.go` | Patch-send form (repo, range, to, cc, preview) |
| `internal/send/send.go` | `ApplyPatchCmd`, `CommitPatchCmd`, `SendPatchCmd`, `SendRawPatchCmd` |
| `app/app.go` | Orchestrates TUI messages between email viewer, patch-send form, and send commands |

---

## Receiving & Viewing Patches

### Detection

When an email is opened, `view.DetectPatch()` checks whether the body contains
a git patch:

1. **MIME gate** — only `text/plain` bodies are checked. HTML emails are
   rejected immediately.
2. **Content sniff** — looks for `diff --git` in the body. This is the primary
   signal, not content-type headers or subject alone.
3. **CRLF normalization** — `\r` is stripped from `\r\n` line endings before
   parsing. This is critical: `go-mailpatch`'s body/diff separator (`---`)
   detection fails on `\r\n`, which would cause the diffstat block to leak
   into the commit message.
4. **Subject prefix parsing** — `parseSubjectPrefix()` extracts series
   metadata from `[PATCH]`, `[PATCH v2]`, `[PATCH 1/3]`, `[RFC PATCH v3 2/5]`,
   etc., yielding the clean subject, series index, series total, and version.

### Rendering

If a patch is detected, the email viewer renders a rich layout:

- **Patch badge** — a `[📮 Patch]` badge appears in the email header.
- **Metadata banner** — a bordered box showing "📮 Git Patch", the subject,
  author, series info (e.g. `3/5 v2`), and a colorized diffstat summary
  ("Files: N changed, N insertions(+), N deletions(-)").
- **Commit message** — rendered as plain text below the banner.
- **Diffstat block** — each file entry is colorized: file paths in secondary
  color, `+` in green, `-` in red.
- **Diff section** — rendered by `diffview.RenderUnified()` as full-width
  solid blocks with background colors per line type (green = added, red =
  deleted, dark gray = context), line-number columns, and per-language syntax
  highlighting (Go, Python, JS/TS, Rust, C/C++, Java, Ruby, Bash, HTML, CSS,
  JSON, YAML, SQL, Markdown — detected from file extension).

The help bar shows `p: apply patch` only when the email is a patch with an
actual diff (not a cover letter).

---

## Applying Patches

### From the TUI (`p` key)

Press `p` in the email viewer when viewing a patch:

1. **Apply to working tree** — `internal/send.ApplyPatchCmd()` strips CRLF,
   splits the body from the diff via `mailpatch.SplitBodyDiff()`, parses the
   diff into structured `FileChange` objects, then calls
   `patchapply.Apply()` on a `DirFS` confined to the current directory.
   Changes are **transactional per file**: every hunk is matched in memory
   before anything is written. If a hunk conflicts, the file is left
   untouched.
2. **Stage** — `git add -A` stages all changes.
3. **Commit** — `CommitPatchCmd()` builds a commit message by stripping the
   `[PATCH...]` prefix from the subject and appending the commit message body
   (including trailers like `Signed-off-by`, `Co-developed-by`). The original
   author's name and email are extracted from the `From:` header and passed as
   `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL`. The commit runs via
   `tea.ExecProcess`, which releases the terminal so GPG pinentry can run for
   signed commits, then restores the TUI.

### From the CLI (`matcha apply`)

```bash
matcha apply [patch-file] [flags]
```

The CLI apply **never runs git** — it only writes files. No commit, no index,
no HEAD movement. See the [CLI docs](../Features/CLI.md#matcha-apply) for
flags and examples.

---

## Sending Patches

### From the TUI (`P` key)

Press `P` in the email viewer to open the patch-send form:

- **Fields**: repo path, commit range, To, Cc.
- **Preview** — press `ctrl+p` to generate the patch and preview the output
  before sending.
- **Send** — generates the patch via `git format-patch --stdout`, rewrites the
  `From:` header to your account's sending identity, collects recipients from
  the patch headers + form fields, and delivers via SMTP.

### From the CLI (`matcha send-patch`)

```bash
matcha send-patch [flags]
```

See the [CLI docs](../Features/CLI.md#matcha-send-patch) for flags and
examples.

### From-rewriting

SMTP servers reject messages whose `From:` header doesn't match the
authenticated account. Matcha rewrites the `From:` header to your configured
account's sending identity before sending. The original git author is
preserved in the patch body's `From:` line — this is standard `git
format-patch` behavior, so the recipient still sees the true author.

---

## Detection Summary

| Signal | Where | How |
|--------|-------|-----|
| MIME type | `view/patch.go` | Must be `text/plain`; HTML rejected |
| Body content | `view/patch.go` | `diff --git` presence — primary detection |
| Subject prefix | `view/patch.go` (`parseSubjectPrefix`) | Parses `[PATCH]`, `[RFC PATCH]`, `[PATCH vN]`, `[PATCH n/m]` for series metadata |
| Full RFC 5322 parse | `gitmail/gitmail.go` (`IsPatch`) | `mailpatch.ParseBytes()` + `HasDiff()` — used for CLI |
| CRLF handling | `view/patch.go`, `internal/send/send.go` | `\r` stripped before `SplitBodyDiff` to prevent diffstat leaking into commit message |

No content-type header inspection is done at the email protocol level —
detection is purely body-content-based (`diff --git` presence) gated by MIME
type being plain text.

---

## Keybindings

| Key | Context | Action |
|-----|---------|--------|
| `p` | Email viewer (patch email) | Apply patch to working tree + commit |
| `P` | Email viewer (any email) | Open patch-send form |
| `ctrl+p` | Patch-send form | Preview generated patch |

These can be customized via `apply_patch` and `send_patch` in your
[keybindings config](../Features/Keybinds.md).
