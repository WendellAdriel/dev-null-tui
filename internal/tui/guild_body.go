package tui

import (
	"net/url"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// guildChip draws a guild as its icon, or its emoji until the icon is ready, and its name, marked so a click opens
// the board.
func guildChip(guild api.GuildBadge, p palette, icons *graphics.Manager) string {
	if icon := guildChipIcon(guild, icons); icon != "" {
		return guildMark(guild.Handle) + icon + text(p.blue).Bold(true).Render(" "+safeText(guild.Name)) + guildEnd
	}
	return guildPaint(guild.Handle, p)(guildLabel(guild))
}

// guildChipRequest is a guild's uploaded icon in the two cells its emoji takes, the same request as the sidebar's.
func guildChipRequest(guild api.GuildBadge) graphics.Request {
	return graphics.Request{URL: graphics.AvatarURL(guild.IconURL), Ring: graphics.NoRing, Cols: 2, Rows: 1, Square: true}
}

func guildChipIcon(guild api.GuildBadge, icons *graphics.Manager) string {
	if guild.IconURL == "" || icons == nil {
		return ""
	}
	return icons.Row(guildChipRequest(guild), 0)
}

func guildMark(handle string) string { return guildStart + url.PathEscape(handle) + "\x1b\\" }

// guildPaint styles text as a guild chip, marked so a click opens that guild's board.
func guildPaint(handle string, p palette) func(...string) string {
	mark, chip := guildMark(handle), text(p.blue).Bold(true).Render
	return func(s ...string) string { return mark + chip(s...) + guildEnd }
}

// guildLine is the web's line under the author: why Following shows a commit, or the board a post went to.
func guildLine(commit api.Commit, p palette, icons *graphics.Manager) string {
	if commit.JoinedGuild != "" {
		guild := api.GuildBadge{Handle: commit.JoinedGuild, Name: "#" + commit.JoinedGuild}
		if commit.Guild != nil && commit.Guild.Handle == commit.JoinedGuild {
			guild = *commit.Guild
		} else if mention, ok := commit.GuildMentions[commit.JoinedGuild]; ok {
			guild = mention
		}
		return text(p.muted).Render("because you joined ") + guildChip(guild, p, icons)
	}
	if commit.Guild == nil {
		return ""
	}
	return text(p.muted).Render("posted in ") + guildChip(*commit.Guild, p, icons) + text(p.muted).Render(" · not shown in general logs")
}
