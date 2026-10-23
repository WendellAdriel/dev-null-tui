package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Help    key.Binding
	Dismiss key.Binding
	Quit    key.Binding
}

func newKeys() keyMap {
	return keyMap{
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Dismiss: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close help")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Help, k.Dismiss, k.Quit}}
}
