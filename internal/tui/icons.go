package tui

type iconSet struct {
	company, location, link, github, joined, log, notifications, profile, commit, starred, lock string
}

// Nerd Font glyphs mirror the web's Lucide icons. Plain glyphs work with any font.
var (
	nerdIcons  = iconSet{company: "", location: "", link: "", github: "", joined: "", log: "", notifications: "", profile: "", commit: "", starred: "", lock: ""}
	plainIcons = iconSet{company: "⌂", location: "⌖", link: "↗", github: "⎇", joined: "◷", log: "◉", notifications: "◔", profile: "◎", commit: "＋", starred: "☆", lock: "⚿"}
)

// WithIcons picks the icon set: "plain" for any font, otherwise Nerd Font glyphs.
func (m AppModel) WithIcons(set string) AppModel {
	m.icons = nerdIcons
	if set == "plain" {
		m.icons = plainIcons
	}
	return m
}
