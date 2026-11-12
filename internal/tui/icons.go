package tui

type iconSet struct {
	company, location, link, github, gitlab, codeberg, joined, log, notifications, profile, commit, starred, lock, settings, remotes, pin, muted, scheduled, drafts, follow, flame string
}

// Nerd Font glyphs mirror the web's Lucide icons. Plain glyphs work with any font.
var (
	nerdIcons  = iconSet{company: "", location: "", link: "", github: "", gitlab: "\uf296", codeberg: "\uf330", joined: "", log: "", notifications: "", profile: "", commit: "", starred: "", lock: "", settings: "", remotes: "\uf086", pin: "\uf435", muted: "\uf1f6", scheduled: "\uf252", drafts: "\uf044", follow: "\uf234", flame: "\uf06d"}
	plainIcons = iconSet{company: "⌂", location: "⌖", link: "↗", github: "⎇", gitlab: "⎇", codeberg: "⎇", joined: "◷", log: "◉", notifications: "◔", profile: "◎", commit: "＋", starred: "☆", lock: "⚿", settings: "⚙", remotes: "✉", pin: "⚲", muted: "⊘", scheduled: "⧗", drafts: "✎", follow: "+", flame: "∆"}
)

// WithIcons picks the icon set: "plain" for any font, otherwise Nerd Font glyphs.
func (m AppModel) WithIcons(set string) AppModel {
	m.icons = nerdIcons
	if set == "plain" {
		m.icons = plainIcons
	}
	return m
}
