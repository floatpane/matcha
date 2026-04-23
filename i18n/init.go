package i18n

// Package i18n provides internationalization support for the matcha email client.
//
// Usage:
//   import "github.com/floatpane/matcha/i18n"
//   import _ "github.com/floatpane/matcha/i18n/languages" // Register all languages
//
//   func main() {
//       // Initialize i18n
//       if err := i18n.Init("en"); err != nil {
//           log.Fatal(err)
//       }
//
//       // Set language (optional, can also be done via config)
//       i18n.GetManager().SetLanguage("es")
//
//       // Translate
//       text := i18n.GetManager().T("composer.title")
//   }
