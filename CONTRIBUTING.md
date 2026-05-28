# Contributing to Matcha

Thank you for your interest in contributing to Matcha! This guide will help you get started.

## Getting Started

### Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- A terminal emulator with modern capabilities (kitty, ghostty, alacritty, etc.)
- An IMAP email account for testing

### Setup

```bash
git clone https://github.com/floatpane/matcha.git
cd matcha
go mod tidy
```

### Build & Run

```bash
make build     # builds to bin/matcha
make run       # builds and runs in one step
```

### Testing

```bash
make test              # run all tests
make test-verbose      # run tests with verbose output
make test-coverage     # run tests and generate a coverage report
```

### Linting

```bash
make lint       # runs go fmt and go vet
```

## Making Changes

### Branch Naming

Create a branch from `master` using one of these prefixes:

- `feature/` — new functionality
- `fix/` — bug fixes
- `docs/` — documentation changes
- `refactor/` — code restructuring without behavior changes

### Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/). Format your commit messages as:

```
type(scope): short description
```

Common types: `feat`, `fix`, `docs`, `test`, `ci`, `chore`.

Keep commit messages to 40 characters or fewer.

Examples:
```
feat(compose): add CC/BCC field support
fix(imap): handle connection timeout gracefully
docs: update installation instructions
```

### Before Submitting a PR

1. Run `make lint` and fix any issues.
2. Run `make test` and make sure all tests pass.
3. Keep your PR focused — one logical change per PR.
4. Write a clear PR description explaining **what** changed and **why**.

## Reporting Bugs

Open an issue using the [bug report template](https://github.com/floatpane/matcha/issues/new?template=bug_report.md). Include:

- Steps to reproduce the issue
- Expected vs. actual behavior
- Your OS, terminal emulator, and Matcha version (`matcha --version`)

## Requesting Features

Open an issue using the [feature request template](https://github.com/floatpane/matcha/issues/new?template=feature_request.md) with a clear description of the problem you're trying to solve and your proposed solution.

## AI Policy

We welcome contributions that use AI-assisted tools (Copilot, Claude, ChatGPT, etc.) as part of the development process. That said, contributors are fully responsible for any code they submit, regardless of how it was written.

**What we expect:**

- **Understand what you submit.** You should be able to explain every line of your PR. If you can't explain it, don't submit it.
- **Review AI output carefully.** AI tools can produce plausible-looking code that is subtly wrong, insecure, or doesn't match the project's patterns. Treat AI suggestions the same way you'd treat a Stack Overflow snippet — verify before committing.
- **Don't submit AI-generated issues, reviews, or comments.** Discussions should be genuine human communication. Using AI to help draft something is fine, but don't paste raw AI output into issues or review comments.
- **No AI-generated tests that don't actually test anything.** Tests must be meaningful and actually validate behavior, not just exist for coverage numbers.
- **Attribute when appropriate.** If a significant portion of your contribution was AI-assisted, a brief mention in your PR description is appreciated but not required.

**What we won't accept:**

- Bulk PRs of AI-generated refactors, documentation, or "improvements" that weren't requested.
- Code that introduces hallucinated dependencies, APIs, or patterns that don't exist in the project.
- Contributions where the author clearly doesn't understand the changes they're proposing.

The goal is simple: AI is a tool. Use it well, take ownership of the output, and make sure your contribution actually improves the project.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold a welcoming and respectful environment for everyone.
