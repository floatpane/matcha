# i18n - Custom Internationalization Library

Custom-built internationalization system for matcha email client. Zero external i18n dependencies - everything implemented from scratch.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Directory Structure](#directory-structure)
- [Core Components](#core-components)
  - [Translation Manager](#translation-manager)
  - [Bundle System](#bundle-system)
  - [Localizer](#localizer)
  - [Plural Rules Engine](#plural-rules-engine)
  - [Template System](#template-system)
  - [Language Registry](#language-registry)
- [Data Flow](#data-flow)
- [Translation File Format](#translation-file-format)
- [Plural Forms](#plural-forms)
- [Caching Strategy](#caching-strategy)
- [Language Detection](#language-detection)
- [Adding/Editing Languages](#addingediting-languages)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                      │
│              (TUI components call t(), tn(), tpl())         │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    Global Manager                           │
│  • Singleton instance                                       │
│  • Current language state                                   │
│  • Localizer instances                                      │
└──────────────────────────┬──────────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
┌────────▼────────┐ ┌─────▼──────┐ ┌───────▼────────┐
│     Bundle      │ │  Localizer │ │     Cache      │
│  All messages   │ │  Per-lang  │ │  Translations  │
│  All locales    │ │  instance  │ │  Rendered text │
└────────┬────────┘ └─────┬──────┘ └────────────────┘
         │                │
         │         ┌──────▼──────────┐
         │         │   Pluralizer    │
         │         │  Plural rules   │
         │         └─────────────────┘
         │
┌────────▼────────────────────────────────────────────────────┐
│                   Translation Files                         │
│            locales/en.json, locales/uk.json, ...            │
└─────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
i18n/
├── manager.go           # Global singleton manager
├── bundle.go            # Translation bundle storage
├── locale.go            # Locale representation
├── message.go           # Message structure (with plural forms)
├── localizer.go         # Per-language translation engine
├── loader.go            # Load & embed translation files
├── parser.go            # JSON parser for translation files
├── plural_rules.go      # Plural rule functions for all languages
├── pluralizer.go        # Plural form selection logic
├── template.go          # Simple {var} template engine
├── interpolator.go      # Variable interpolation in strings
├── cache.go             # In-memory translation cache
├── fallback.go          # Fallback chain (uk → en)
├── detector.go          # Auto-detect user language
├── registry.go          # Available languages registry
├── formatter.go         # Number formatting per locale
├── date_formatter.go    # Date/time formatting per locale
├── context.go           # Context keys for locale passing
├── errors.go            # Error types
├── init.go              # Package initialization
├── embed.go             # Embed translation files in binary
├── languages/           # Language-specific implementations
│   ├── base.go          # Base language interface
│   ├── en.go            # English locale registration
│   ├── uk.go            # Ukrainian locale registration
│   ├── es.go            # Spanish locale registration
│   └── ...              # Other languages
└── locales/             # Translation JSON files
    ├── en.json          # English translations (base)
    ├── uk.json          # Ukrainian translations
    └── ...              # Other language files
```

## Core Components

### Translation Manager

**File:** `manager.go`

Global singleton that coordinates all i18n operations. Provides three main translation functions:

- `T(key)` - Simple translation lookup
- `Tn(key, count, data)` - Translation with pluralization
- `Tpl(key, data)` - Translation with template variables

```go
// Usage in TUI components
import "github.com/floatpane/matcha/i18n"

func render() string {
    title := i18n.GetManager().T("composer.title")
    count := i18n.GetManager().Tn("inbox.unread", 5, map[string]interface{}{
        "count": 5,
    })
    return title + " - " + count
}
```

Maintains:
- Current active language
- Bundle with all loaded translations
- Localizer instances per language
- Translation cache

### Bundle System

**File:** `bundle.go`

Central storage for all translation messages across all languages. Thread-safe with RWMutex.

```go
type Bundle struct {
    defaultLang string
    messages    map[string]MessageMap  // lang → message ID → Message
    locales     map[string]*Locale     // lang → Locale info
    mu          sync.RWMutex
}
```

Responsibilities:
- Store translations for all languages
- Retrieve messages by language + key
- Register locale definitions
- List available languages

### Localizer

**File:** `localizer.go`

Per-language translation engine. Each active language gets one localizer instance.

```go
type Localizer struct {
    lang    string
    bundle  *Bundle
    locale  *Locale
    cache   *Cache
}
```

Core methods:
- `Localize(messageID)` - Basic lookup
- `LocalizePlural(messageID, count, data)` - With plural rules
- `LocalizeTemplate(messageID, data)` - With variable substitution

Handles:
- Message lookup with fallback
- Plural form selection
- Template interpolation
- Result caching

### Plural Rules Engine

**Files:** `plural_rules.go`, `pluralizer.go`

Implements CLDR plural rules for all supported languages.

```go
type PluralForm int

const (
    Zero PluralForm = iota  // 0 items
    One                     // 1 item
    Two                     // 2 items (Arabic, Welsh)
    Few                     // 2-4 items (Slavic languages)
    Many                    // 5+ items (Slavic languages)
    Other                   // Default/fallback
)
```

Each language has specific rules:

**English/Spanish:** Simple (one/other)
```go
func EnglishPlural(n int) PluralForm {
    if n == 1 {
        return One
    }
    return Other
}
```

**Ukrainian/Russian:** Complex (one/few/many)
```go
func UkrainianPlural(n int) PluralForm {
    mod10 := n % 10
    mod100 := n % 100
    
    if mod10 == 1 && mod100 != 11 {
        return One     // 1, 21, 31, 41...
    }
    if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
        return Few     // 2-4, 22-24, 32-34...
    }
    return Many        // 0, 5-20, 25-30...
}
```

**Arabic:** Very complex (zero/one/two/few/many/other)

### Template System

**Files:** `template.go`, `interpolator.go`

Simple variable substitution using `{variable}` syntax.

```go
template := "Update available: {latest} (current: {current})"
data := map[string]interface{}{
    "latest": "1.5.0",
    "current": "1.4.0",
}
result := Interpolate(template, data)
// → "Update available: 1.5.0 (current: 1.4.0)"
```

**Design philosophy:** Keep simple - no complex logic, just variable replacement. Complex formatting done in Go code before passing to templates.

### Language Registry

**File:** `registry.go`

Global registry of all available languages. Languages self-register on init.

```go
// In languages/uk.go
func init() {
    i18n.RegisterLanguage(&i18n.Locale{
        Code:       "uk",
        Name:       "Ukrainian",
        NativeName: "Українська",
        Direction:  "ltr",
        PluralFunc: i18n.UkrainianPlural,
    })
}
```

Registry provides:
- `GetLanguage(code)` - Lookup locale by code
- `AvailableLanguages()` - List all registered locales
- `LanguageCodes()` - List all language codes

## Data Flow

### Translation Lookup Flow

```
1. TUI calls t("composer.title")
              ↓
2. Manager.T() → GetLocalizer(currentLang)
              ↓
3. Localizer checks cache
   • Cache hit? → return cached value
   • Cache miss? → continue
              ↓
4. Bundle.GetMessage(lang, "composer.title")
   • Found? → return Message
   • Not found? → try fallback chain (uk → en)
              ↓
5. Return message.One (singular form)
              ↓
6. Cache result for future lookups
              ↓
7. Return translated string to TUI
```

### Pluralization Flow

```
1. TUI calls tn("inbox.unread", 5, {"count": 5})
              ↓
2. Manager.Tn() → GetLocalizer(currentLang)
              ↓
3. Get Message from bundle
              ↓
4. Call locale.PluralFunc(5)
   • English: 5 → Other
   • Ukrainian: 5 → Many
   • Arabic: 5 → Few
              ↓
5. Select appropriate form from Message:
   • message.One (n=1)
   • message.Few (n=2-4 in Ukrainian)
   • message.Many (n=5+ in Ukrainian)
   • message.Other (fallback)
              ↓
6. Interpolate {count} → "5"
              ↓
7. Return "5 листів" (Ukrainian) or "5 emails" (English)
```

## Translation File Format

**Location:** `locales/*.json`

**Structure:**

```json
{
  "language": "uk",
  "messages": {
    "common": {
      "yes": "Так",
      "no": "Ні",
      "save": "Зберегти"
    },
    "composer": {
      "title": "Написати новий лист",
      "send": "Надіслати",
      "from": "Від"
    },
    "inbox": {
      "unread": {
        "one": "{count} непрочитаний лист",
        "few": "{count} непрочитані листи",
        "other": "{count} непрочитаних листів"
      }
    }
  }
}
```

**Nested structure:** Dot notation for keys (`composer.title`, `inbox.unread`)

**Plural objects:** When value is object with `one`/`few`/`many`/`other`, it's treated as plural form.

## Plural Forms

Languages use different plural categories:

| Language | Forms Used | Example (emails) |
|----------|-----------|------------------|
| English | one, other | 1 email, 2 emails |
| Spanish | one, other | 1 correo, 2 correos |
| German | one, other | 1 E-Mail, 2 E-Mails |
| French | one, other | 0 courriel, 1 courriel, 2 courriels |
| Ukrainian | one, few, other | 1 лист, 2 листи, 5 листів |
| Russian | one, few, other | 1 письмо, 2 письма, 5 писем |
| Polish | one, few, many, other | 1 e-mail, 2 e-maile, 5 e-maili |
| Arabic | zero, one, two, few, many, other | Complex rules |
| Japanese | other | All numbers use same form |
| Chinese | other | All numbers use same form |

## Caching Strategy

**File:** `cache.go`

Simple in-memory string cache with RWMutex for thread safety.

**Cache key format:** `{lang}:{messageID}:{count}:{hash(data)}`

**Why cache?**
- Translation lookup involves: map lookups, plural calculation, interpolation
- Typical UI has ~100 translated strings
- Redrawing UI on every keypress = wasted CPU
- Cache hit = instant string return

**Cache invalidation:** Cache cleared when language changes via `SetLanguage()`.

**No size limit:** Translation cache is small (~KB for entire UI) and only grows to unique message combinations actually used.

## Language Detection

**File:** `detector.go`

Detection priority:

1. **Config file:** `language = "uk"` in `~/.config/matcha/config.toml`
2. **Environment:** `LANG`, `LC_ALL`, `LC_MESSAGES`
3. **System locale:** OS-specific detection
4. **Default:** Falls back to `"en"`

```go
func DetectLanguage(cfg *config.Config) string {
    // 1. Check config
    if cfg.Language != "" && isValidLanguage(cfg.Language) {
        return cfg.Language
    }
    
    // 2. Check environment
    if lang := detectFromEnv(); lang != "" {
        return lang
    }
    
    // 3. Check system
    if lang := detectFromSystem(); lang != "" {
        return lang
    }
    
    // 4. Default
    return "en"
}
```

Normalizes language codes: `en_US.UTF-8` → `en`, `uk_UA` → `uk`

## Adding/Editing Languages

### Adding a New Language

**1. Create translation file:**

```bash
cd i18n/locales
cp en.json xx.json  # Replace 'xx' with language code (ISO 639-1)
```

**2. Update language code:**

```json
{
  "language": "xx",
  "messages": { ... }
}
```

**3. Translate all strings:**

- Keep JSON structure identical to `en.json`
- Translate all message values
- Preserve placeholders: `{count}`, `{latest}`, `{current}`
- Don't translate technical terms: S/MIME, PGP, IMAP, SMTP
- Don't translate commands: `matcha update`

**4. Handle plural forms:**

Check plural rules for your language at [CLDR Plural Rules](https://cldr.unicode.org/index/cldr-spec/plural-rules).

Add plural forms as needed:

```json
"hours_ago": {
  "one": "{count} hour ago",
  "other": "{count} hours ago"
}
```

For complex plurals (Ukrainian, Arabic, Polish):

```json
"address_count": {
  "one": "{count} адреса",
  "few": "{count} адреси",
  "other": "{count} адрес"
}
```

**5. Register language (if not already in `languages/`):**

Create `languages/xx.go`:

```go
package languages

import "github.com/floatpane/matcha/i18n"

func init() {
    i18n.RegisterLanguage(&i18n.Locale{
        Code:       "xx",
        Name:       "YourLanguage",
        NativeName: "YourLanguageInNativeScript",
        Direction:  "ltr",  // or "rtl" for Arabic, Hebrew, etc.
        PluralFunc: i18n.YourLanguagePlural,
    })
}
```

If plural function doesn't exist in `plural_rules.go`, add it:

```go
// In plural_rules.go
func YourLanguagePlural(n int) PluralForm {
    // Implement CLDR rules for your language
    if n == 1 {
        return One
    }
    return Other
}
```

**6. Test:**

```bash
go build
# Edit config: language = "xx"
./matcha
```

Verify:
- All UI elements translated
- Plural forms work (test with 0, 1, 2, 5, 21 items)
- Variables interpolate correctly
- No English leaks through

### Editing Existing Translations

**1. Open translation file:**

```bash
vi i18n/locales/uk.json
```

**2. Find key using dot notation:**

Translation keys follow UI structure:
- `composer.*` - Email composer
- `inbox.*` - Inbox view  
- `settings_general.*` - General settings
- `common.*` - Shared elements

**3. Update translation:**

```json
{
  "composer": {
    "title": "Old translation"  // Change this
  }
}
```

**4. Rebuild and test:**

```bash
go build
./matcha
```

Language changes apply instantly (no restart needed).

### Translation Guidelines

**Do translate:**
- All visible UI text
- Button labels
- Menu items
- Help text
- Tips
- Error messages shown to user
- Status messages

**Don't translate:**
- Backend error logs
- Debug messages
- Protocol names (IMAP, SMTP, S/MIME, PGP)
- File paths (`~/.config/matcha/`)
- Environment variables (`$EDITOR`)
- Commands (`matcha update`)
- Technical identifiers

**Variable placeholders:**

Always keep variables unchanged:

```json
// ✅ Correct
"update_available": "Доступне оновлення: {latest} (встановлено: {current})"

// ❌ Wrong - renamed variable
"update_available": "Доступне оновлення: {останній} (встановлено: {поточний})"
```

### Testing Checklist

- [ ] All screens display in target language
- [ ] Test plural forms with different counts (0, 1, 2, 5, 21)
- [ ] Variables interpolate correctly
- [ ] No untranslated English text (except technical terms)
- [ ] Text fits in UI (not truncated)
- [ ] Special characters render properly
- [ ] RTL languages display correctly (Arabic, Hebrew)
- [ ] Date/time formats work
- [ ] Help text makes sense in context

### Contributing Translations

1. Fork repository
2. Add/edit translation file in `i18n/locales/`
3. Test thoroughly with checklist above
4. Submit pull request with:
   - Translation file
   - Screenshots of translated UI
   - Note about plural form testing
   - List any technical challenges

**Translation quality > completeness.** Better to have 80% high-quality translation than 100% machine-translated text.

---

**For more details on using translations in code, see:** [Documentation](https://docs.matcha.floatpane.com/localization).
