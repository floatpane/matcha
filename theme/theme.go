package theme

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme defines the color palette for the application.
type Theme struct {
	Name       string      `json:"name"`
	Accent     color.Color `json:"-"`
	AccentDark color.Color `json:"-"`
	AccentText color.Color `json:"-"`
	Secondary  color.Color `json:"-"`
	SubtleText color.Color `json:"-"`
	MutedText  color.Color `json:"-"`
	DimText    color.Color `json:"-"`
	Danger     color.Color `json:"-"`
	Warning    color.Color `json:"-"`
	Tip        color.Color `json:"-"`
	Link       color.Color `json:"-"`
	Directory  color.Color `json:"-"`
	Contrast   color.Color `json:"-"`
}

// themeJSON is the JSON-serializable form of Theme using string color values.
type themeJSON struct {
	Name       string `json:"name"`
	Accent     string `json:"accent"`
	AccentDark string `json:"accent_dark"`
	AccentText string `json:"accent_text"`
	Secondary  string `json:"secondary"`
	SubtleText string `json:"subtle_text"`
	MutedText  string `json:"muted_text"`
	DimText    string `json:"dim_text"`
	Danger     string `json:"danger"`
	Warning    string `json:"warning"`
	Tip        string `json:"tip"`
	Link       string `json:"link"`
	Directory  string `json:"directory"`
	Contrast   string `json:"contrast"`
}

func themeFromJSON(j themeJSON) Theme {
	return Theme{
		Name:       j.Name,
		Accent:     lipgloss.Color(j.Accent),
		AccentDark: lipgloss.Color(j.AccentDark),
		AccentText: lipgloss.Color(j.AccentText),
		Secondary:  lipgloss.Color(j.Secondary),
		SubtleText: lipgloss.Color(j.SubtleText),
		MutedText:  lipgloss.Color(j.MutedText),
		DimText:    lipgloss.Color(j.DimText),
		Danger:     lipgloss.Color(j.Danger),
		Warning:    lipgloss.Color(j.Warning),
		Tip:        lipgloss.Color(j.Tip),
		Link:       lipgloss.Color(j.Link),
		Directory:  lipgloss.Color(j.Directory),
		Contrast:   lipgloss.Color(j.Contrast),
	}
}

// Built-in themes

var Matcha = Theme{
	Name:       "Matcha",
	Accent:     lipgloss.Color("42"),
	AccentDark: lipgloss.Color("#25A065"),
	AccentText: lipgloss.Color("#FFFDF5"),
	Secondary:  lipgloss.Color("244"),
	SubtleText: lipgloss.Color("245"),
	MutedText:  lipgloss.Color("247"),
	DimText:    lipgloss.Color("250"),
	Danger:     lipgloss.Color("196"),
	Warning:    lipgloss.Color("208"),
	Tip:        lipgloss.Color("214"),
	Link:       lipgloss.Color("#9BC4FF"),
	Directory:  lipgloss.Color("34"),
	Contrast:   lipgloss.Color("#000000"),
}

var Rose = Theme{
	Name:       "Rose",
	Accent:     lipgloss.Color("#E8729B"),
	AccentDark: lipgloss.Color("#B5547A"),
	AccentText: lipgloss.Color("#FFFDF5"),
	Secondary:  lipgloss.Color("244"),
	SubtleText: lipgloss.Color("245"),
	MutedText:  lipgloss.Color("247"),
	DimText:    lipgloss.Color("250"),
	Danger:     lipgloss.Color("196"),
	Warning:    lipgloss.Color("208"),
	Tip:        lipgloss.Color("214"),
	Link:       lipgloss.Color("#9BC4FF"),
	Directory:  lipgloss.Color("#E8729B"),
	Contrast:   lipgloss.Color("#000000"),
}

var Lavender = Theme{
	Name:       "Lavender",
	Accent:     lipgloss.Color("#B4A7D6"),
	AccentDark: lipgloss.Color("#8E7CC3"),
	AccentText: lipgloss.Color("#FFFDF5"),
	Secondary:  lipgloss.Color("244"),
	SubtleText: lipgloss.Color("245"),
	MutedText:  lipgloss.Color("247"),
	DimText:    lipgloss.Color("250"),
	Danger:     lipgloss.Color("196"),
	Warning:    lipgloss.Color("208"),
	Tip:        lipgloss.Color("214"),
	Link:       lipgloss.Color("#9BC4FF"),
	Directory:  lipgloss.Color("#B4A7D6"),
	Contrast:   lipgloss.Color("#000000"),
}

var Ocean = Theme{
	Name:       "Ocean",
	Accent:     lipgloss.Color("#5B9BD5"),
	AccentDark: lipgloss.Color("#3A7BBF"),
	AccentText: lipgloss.Color("#FFFDF5"),
	Secondary:  lipgloss.Color("244"),
	SubtleText: lipgloss.Color("245"),
	MutedText:  lipgloss.Color("247"),
	DimText:    lipgloss.Color("250"),
	Danger:     lipgloss.Color("196"),
	Warning:    lipgloss.Color("208"),
	Tip:        lipgloss.Color("214"),
	Link:       lipgloss.Color("#9BC4FF"),
	Directory:  lipgloss.Color("#5B9BD5"),
	Contrast:   lipgloss.Color("#000000"),
}

var Peach = Theme{
	Name:       "Peach",
	Accent:     lipgloss.Color("#FAB387"),
	AccentDark: lipgloss.Color("#E0956E"),
	AccentText: lipgloss.Color("#1E1E2E"),
	Secondary:  lipgloss.Color("244"),
	SubtleText: lipgloss.Color("245"),
	MutedText:  lipgloss.Color("247"),
	DimText:    lipgloss.Color("250"),
	Danger:     lipgloss.Color("#F38BA8"),
	Warning:    lipgloss.Color("#F9E2AF"),
	Tip:        lipgloss.Color("#F9E2AF"),
	Link:       lipgloss.Color("#89B4FA"),
	Directory:  lipgloss.Color("#FAB387"),
	Contrast:   lipgloss.Color("#1E1E2E"),
}

var CatppuccinMocha = Theme{
	Name:       "Catppuccin Mocha",
	Accent:     lipgloss.Color("#89B4FA"),
	AccentDark: lipgloss.Color("#74C7EC"),
	AccentText: lipgloss.Color("#1E1E2E"),
	Secondary:  lipgloss.Color("#6C7086"),
	SubtleText: lipgloss.Color("#7F849C"),
	MutedText:  lipgloss.Color("#9399B2"),
	DimText:    lipgloss.Color("#BAC2DE"),
	Danger:     lipgloss.Color("#F38BA8"),
	Warning:    lipgloss.Color("#FAB387"),
	Tip:        lipgloss.Color("#F9E2AF"),
	Link:       lipgloss.Color("#89DCEB"),
	Directory:  lipgloss.Color("#89B4FA"),
	Contrast:   lipgloss.Color("#1E1E2E"),
}

// BuiltinThemes lists all built-in themes in display order.
var BuiltinThemes = []Theme{
	Matcha,
	Rose,
	Lavender,
	Ocean,
	Peach,
	CatppuccinMocha,
}

// ActiveTheme is the currently active theme used for styling.
var ActiveTheme = Matcha

// SetTheme sets the active theme by name. Returns true if found.
// It searches built-in themes first, then custom themes.
func SetTheme(name string) bool {
	if name == "" {
		ActiveTheme = Matcha
		return true
	}
	for _, t := range BuiltinThemes {
		if strings.EqualFold(t.Name, name) {
			ActiveTheme = t
			return true
		}
	}
	// Try custom themes
	custom := LoadCustomThemes()
	for _, t := range custom {
		if strings.EqualFold(t.Name, name) {
			ActiveTheme = t
			return true
		}
	}
	return false
}

// AllThemes returns all available themes (built-in + custom).
func AllThemes() []Theme {
	all := make([]Theme, len(BuiltinThemes))
	copy(all, BuiltinThemes)
	all = append(all, LoadCustomThemes()...)
	return all
}

// LoadCustomThemes loads custom themes from ~/.config/matcha/themes/*.json.
func LoadCustomThemes() []Theme {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	themesDir := filepath.Join(home, ".config", "matcha", "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return nil
	}

	var themes []Theme
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(themesDir, entry.Name()))
		if err != nil {
			continue
		}
		var j themeJSON
		if err := json.Unmarshal(data, &j); err != nil {
			continue
		}
		if j.Name == "" {
			j.Name = strings.TrimSuffix(entry.Name(), ".json")
		}
		themes = append(themes, themeFromJSON(j))
	}
	return themes
}
