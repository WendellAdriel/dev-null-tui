package tui

import (
	"net/url"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// guildChip draws a guild as its emoji and name, marked so a click opens the board.
func guildChip(guild api.GuildBadge, p palette) string {
	return guildPaint(guild.Handle, p)(guildLabel(guild))
}

// guildPaint styles text as a guild chip, marked so a click opens that guild's board.
func guildPaint(handle string, p palette) func(...string) string {
	mark, chip := guildStart+url.PathEscape(handle)+"\x1b\\", text(p.blue).Bold(true).Render
	return func(s ...string) string { return mark + chip(s...) + guildEnd }
}

// guildLine is the web's line under the author: why Following shows a commit, or the board a post went to.
func guildLine(commit api.Commit, p palette) string {
	if commit.JoinedGuild != "" {
		guild := api.GuildBadge{Handle: commit.JoinedGuild, Name: "#" + commit.JoinedGuild}
		if commit.Guild != nil && commit.Guild.Handle == commit.JoinedGuild {
			guild = *commit.Guild
		} else if mention, ok := commit.GuildMentions[commit.JoinedGuild]; ok {
			guild = mention
		}
		return text(p.muted).Render("because you joined ") + guildChip(guild, p)
	}
	if commit.Guild == nil {
		return ""
	}
	return text(p.muted).Render("posted in ") + guildChip(*commit.Guild, p) + text(p.muted).Render(" · not shown in general logs")
}
