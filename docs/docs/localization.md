# Localization Guide

Matcha uses a custom i18n (internationalization) system to support multiple languages. This guide explains how to add new translations or edit existing ones.

## Changing Language

Set your preferred language in the config file (`~/.config/matcha/config.json`):

```toml
language = "uk"  # or "es", "de", "fr", etc.
```

Or in Matcha Settings menu → General → Language.

## File Structure

```
i18n/
├── locales/
│   ├── en.json       # English (base)
│   ├── uk.json       # Ukrainian
│   ├── es.json       # Spanish
│   └── ...
└── languages/
    ├── en.go         # English plural rules
    ├── uk.go         # Ukrainian plural rules
    └── ...
```

## Adding a New Translation

### 1. Create Translation File

Copy `i18n/locales/en.json` to `i18n/locales/[lang].json`:

```bash
cp i18n/locales/en.json i18n/locales/es.json
```

### 2. Update Language Code

Change the `language` field:

```json
{
  "language": "es",
  "messages": {
    ...
  }
}
```

### 3. Translate All Strings

Translate all message values while preserving:
- JSON structure
- Placeholder variables: `{count}`, `{latest}`, `{current}`, etc.
- Technical terms: S/MIME, PGP, IMAP, SMTP, etc.
- Commands and file paths

**Example:**

```json
"composer": {
  "title": "Redactar nuevo correo",
  "from": "De",
  "to_placeholder": "Ingrese direcciones de correo de destinatarios.",
  "send": "Enviar"
}
```

### 4. Handle Plural Forms

Different languages have different plural rules. Matcha supports:

- `one` - Singular (1)
- `few` - Few items (2-4 in some languages)
- `many` - Many items (5+ in some languages)
- `other` - Default/all other counts

**English (simple):**
```json
"address_count": {
  "one": "{count} address",
  "other": "{count} addresses"
}
```

**Ukrainian (complex):**
```json
"address_count": {
  "one": "{count} адреса",
  "few": "{count} адреси",
  "other": "{count} адрес"
}
```

**Arabic (very complex):**
```json
"hours_ago": {
  "zero": "منذ {count} ساعة",
  "one": "منذ ساعة واحدة",
  "two": "منذ ساعتين",
  "few": "منذ {count} ساعات",
  "many": "منذ {count} ساعة",
  "other": "منذ {count} ساعة"
}
```

### 5. Register Language (Optional)

If adding a completely new language not in `i18n/languages/`, create the plural rules file:

**i18n/languages/es.go:**
```go
package languages

import "github.com/floatpane/matcha/i18n"

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Code:       "es",
		Name:       "Spanish",
		NativeName: "Español",
		Direction:  "ltr",
		PluralFunc: i18n.SpanishPlural,
	})
}
```

Plural function already exists in `i18n/plural_rules.go` for common languages.

### 6. Test Translation

1. Build matcha: `go build`
2. Set language in config: `language = "es"`
3. Restart matcha
4. Verify all UI elements display translated text

## Editing Existing Translations

### 1. Find Translation File

Open `i18n/locales/[lang].json` for your language.

### 2. Locate Translation Key

Translation keys follow dot notation matching UI structure:

- `composer.*` - Email composer screen
- `inbox.*` - Inbox view
- `settings.*` - Settings menu
- `settings_general.*` - General settings
- `settings_accounts.*` - Account settings
- `choice.*` - Main menu
- `common.*` - Shared UI elements

**Example key paths:**
```
composer.title
inbox.all_accounts
settings_general.language
settings_encryption.password_label
```

### 3. Update Translation

Edit the string value:

```json
"composer": {
  "title": "Redactar correo nuevo"  // Old
  "title": "Escribir nuevo correo"  // New
}
```

### 4. Rebuild and Test

```bash
go build
./matcha
```

## Translation Guidelines

### Do Translate:
✅ All UI text visible to users  
✅ Help text and tips  
✅ Button labels  
✅ Menu items  
✅ Error messages shown in UI  
✅ Status messages  

### Don't Translate:
❌ Error logs (backend)  
❌ Debug messages  
❌ Protocol names (IMAP, SMTP, PGP, S/MIME)  
❌ File paths  
❌ Environment variables  
❌ Command names (`matcha update`)  
❌ Code/technical identifiers  

### Placeholder Variables

Keep variables intact:

```json
// ✅ Correct
"update_available": "Mise à jour disponible: {latest} (installé: {current})"

// ❌ Wrong - renamed variable
"update_available": "Mise à jour disponible: {derniere} (installé: {actuel})"

// ❌ Wrong - removed variable
"update_available": "Mise à jour disponible (installé)"
```

### Context-Aware Translation

Some keys need context:

```json
// Button in composer
"send": "Enviar"

// Status message
"sent": "Enviado correctamente"

// Different contexts, different translations
```

## Common Translation Keys

### Navigation
```json
"common": {
  "yes": "Sí",
  "no": "No",
  "cancel": "Cancelar",
  "save": "Guardar",
  "delete": "Eliminar",
  "back": "Volver"
}
```

### Relative Time
```json
"inbox": {
  "just_now": "Ahora mismo",
  "minute_ago": {
    "one": "Hace {count} minuto",
    "other": "Hace {count} minutos"
  },
  "hour_ago": {
    "one": "Hace {count} hora",
    "other": "Hace {count} horas"
  }
}
```

## Plural Rules Reference

### English, Spanish, Portuguese
```
one:   1
other: 0, 2-∞
```

### French
```
one:   0, 1
other: 2-∞
```

### German
```
one:   1
other: 0, 2-∞
```

### Russian, Ukrainian
```
one:   1, 21, 31, 41...
few:   2-4, 22-24, 32-34...
other: 0, 5-20, 25-30...
```

### Polish
```
one:   1
few:   2-4, 22-24, 32-34... (not 12-14)
many:  0, 5-21, 25-31...
other: fractions
```

### Arabic
```
zero:  0
one:   1
two:   2
few:   3-10
many:  11-99
other: 100+, fractions
```

### Japanese, Chinese
```
other: all numbers (no plural distinction)
```

## Testing Checklist

When adding/editing translations:

- [ ] All UI screens display in target language
- [ ] Plural forms work correctly (test with 0, 1, 2, 5, 21 items)
- [ ] Variable interpolation works (`{count}`, `{latest}`, etc.)
- [ ] No English text visible (except technical terms)
- [ ] Help text fits in UI (not truncated)
- [ ] Special characters display correctly
- [ ] RTL languages render properly (Arabic)

## Contributing Translations

1. Fork the repository
2. Add/edit translation file in `i18n/locales/`
3. Test thoroughly
4. Submit pull request with:
   - Translation file changes
   - Screenshots showing translated UI
   - Note about plural form testing

## Dynamic Language Switching

Language changes currently require restart. To make dynamic:

1. Save language to config
2. Call `i18n.GetManager().SetLanguage(lang)`
3. Trigger full UI re-render

**Implementation:**

```go
// In settings handler
func (m *Settings) changeLanguage(newLang string) tea.Cmd {
    m.cfg.Language = newLang
    config.SaveConfig(m.cfg)
    i18n.GetManager().SetLanguage(newLang)
    
    // Force complete UI rebuild
    return func() tea.Msg {
        return LanguageChangedMsg{Language: newLang}
    }
}
```

Full dynamic switching requires rebuilding all TUI models with new translations.

## Troubleshooting

### Translation Not Showing

1. Check language code matches file name (`uk.json` → `language = "uk"`)
2. Verify JSON syntax is valid
3. Rebuild: `go build`
4. Clear cache: `rm -rf ~/.cache/matcha`
5. Restart matcha

### Missing Translations

If key missing, falls back to:
1. Base language (English)
2. Translation key itself (e.g., `composer.title`)

Check logs for fallback warnings.

### Plural Forms Not Working

1. Verify plural rules defined for language in `i18n/plural_rules.go`
2. Check JSON structure matches expected forms (`one`, `few`, `many`, `other`)
3. Use `tn()` function in code, not `t()`

## Reference

- Translation files: `i18n/locales/*.json`
- Plural rules: `i18n/plural_rules.go`
- Language registry: `i18n/languages/*.go`
- Unicode CLDR: https://cldr.unicode.org/index/cldr-spec/plural-rules
