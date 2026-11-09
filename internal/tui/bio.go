package tui

import (
	"net/url"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// bioMention is a GitHub account a bio or company tags, drawn as a chip with its avatar.
type bioMention struct {
	handle, href string
	avatar       graphics.Request
}

// bioMentions finds @handles by the web's rule in resources/js/lib/mentions.ts, so emails, "@scope/pkg" and "@vue.js"
// stay text. Each opens its dev/null profile when profiles lists it, else the account on GitHub.
func (m AppModel) bioMentions(value string, profiles map[string]string) ([][2]int, []bioMention) {
	var spans [][2]int
	var mentions []bioMention
	for i := 0; i < len(value); i++ {
		if value[i] != '@' || (i > 0 && endsWord(lastRune(value[:i]))) {
			continue
		}
		end := handleEnd(value, i+1)
		if end == i+1 || !mentionEnds(value[end:]) {
			continue
		}
		handle := value[i+1 : end]
		href := "https://github.com/" + handle
		if profile, ok := profiles[strings.ToLower(handle)]; ok && validHandle(profile) {
			if origin, err := url.Parse(m.account.origin); err == nil && origin.Host != "" {
				href = strings.TrimRight(m.account.origin, "/") + "/@" + profile
			}
		}
		avatar := graphics.Request{URL: "https://github.com/" + handle + ".png?size=40", Cols: 2, Rows: 1}
		spans = append(spans, [2]int{i, end})
		mentions = append(mentions, bioMention{handle: handle, href: href, avatar: avatar})
		i = end - 1
	}
	return spans, mentions
}

// bioPieces draws a bio or company like the web: each tagged account a chip and each #tag a search link.
func (m AppModel) bioPieces(value string, profiles map[string]string, p palette) ([]piece, []graphics.Request) {
	r := bodyRenderer{p: p, style: m.statsStyle(), source: value}
	spans, mentions := m.bioMentions(value, profiles)
	var pieces []piece
	var avatars []graphics.Request
	position := 0
	for i, span := range spans {
		pieces = append(pieces, r.hashtags(value[position:span[0]], position)...)
		pieces = append(pieces, piece{atom: m.bioChip(mentions[i], p)})
		avatars = append(avatars, mentions[i].avatar)
		position = span[1]
	}
	pieces = append(pieces, r.hashtags(value[position:], position)...)
	ink := text(p.ink).Render
	for i := range pieces {
		if pieces[i].atom == "" && pieces[i].paint == nil {
			pieces[i].paint = ink
		}
	}
	return pieces, avatars
}

// bioChip keeps the "@" so a copied bio reads the same, like the web's chip.
func (m AppModel) bioChip(mention bioMention, p palette) string {
	avatar := ""
	if m.images != nil {
		avatar = m.images.Row(mention.avatar, 0)
	}
	if avatar == "" {
		avatar = text(p.muted).Render("◆") + " "
	}
	return linked(mention.href, func(s ...string) string { return strings.Join(s, "") })(avatar + chip("@"+mention.handle, p.ink, p))
}

// bioLines wraps a bio or company to width and lists the avatars its chips draw.
func (m AppModel) bioLines(value string, profiles map[string]string, width int, p palette) ([]string, []graphics.Request) {
	pieces, avatars := m.bioPieces(remoteBody(value), profiles, p)
	return wrapPieces(pieces, width), avatars
}

// bioAvatarsOn lists the avatars of the chips a wrapped bio line draws, so only on-screen lines request them.
func bioAvatarsOn(line string, avatars []graphics.Request) []graphics.Request {
	var shown []graphics.Request
	plain := ansi.Strip(line)
	for _, req := range avatars {
		handle := strings.TrimSuffix(strings.TrimPrefix(req.URL, "https://github.com/"), ".png?size=40")
		if strings.Contains(plain, " @"+handle+" ") {
			shown = append(shown, req)
		}
	}
	return shown
}
