package tui

import (
	"html"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// The body follows the web's display rules in resources/js/components/linkified-text.tsx and commit-body.tsx.
// Go's regexp has no lookaround, so the web's boundary checks are written out by hand.

// piece is a run of body text drawn in one style. An atom is drawn as it is and never split across lines.
type piece struct {
	text  string
	paint func(...string) string
	atom  string
}

func (pc piece) draw(s string) string {
	if pc.paint == nil {
		return s
	}
	return pc.paint(s)
}

type bodyRenderer struct {
	p     palette
	style statsStyle
}

// bodyLines renders a commit body wrapped to width, or unwrapped when width is zero.
func bodyLines(commit api.Commit, width int, p palette, style statsStyle) []string {
	segments := commit.Segments
	if len(commit.Preview) > 0 {
		segments = commit.Preview
	}
	if len(segments) == 0 && commit.Body != "" {
		segments = []api.Segment{{Type: "text", Text: commit.Body}}
	}
	r := bodyRenderer{p, style}
	inner := 0
	if width > 0 {
		inner = max(1, width-2)
	}
	var lines []string
	previousCode := false
	add := func(part []string, code bool) {
		if code || previousCode {
			lines = trimBlankLines(lines)
			part = trimBlankLines(part)
			if len(lines) > 0 && len(part) > 0 {
				lines = append(lines, "")
			}
		}
		lines = append(lines, part...)
		previousCode = code
	}
	for _, block := range quoteBlocks(segments) {
		if block.quoted {
			add(prefixLines(wrapPieces(r.segmentPieces(block.segments), inner), text(p.lanes[5]).Render("▎ ")), false)
			continue
		}
		var run []api.Segment
		for _, segment := range block.segments {
			if segment.Type != "block" {
				run = append(run, segment)
				continue
			}
			if len(run) > 0 {
				add(wrapPieces(r.segmentPieces(run), width), false)
				run = nil
			}
			add(r.codeBlock(segment, inner), true)
		}
		if len(run) > 0 {
			add(wrapPieces(r.segmentPieces(run), width), false)
		}
	}
	return trimBlankLines(lines)
}

// bodyText is the unwrapped body, for excerpts and emptiness checks.
func bodyText(commit api.Commit, p palette, style statsStyle) string {
	return strings.Join(bodyLines(commit, 0, p, style), "\n")
}

func trimBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func prefixLines(lines []string, prefix string) []string {
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return lines
}

type quoteBlock struct {
	quoted   bool
	segments []api.Segment
}

// quoteBlocks splits the body into quoted and unquoted runs of lines, as the web's bodyBlocks does.
func quoteBlocks(segments []api.Segment) []quoteBlock {
	var blocks []quoteBlock
	atLineStart, quoted := true, false
	for _, segment := range segments {
		parts := []api.Segment{segment}
		if segment.Type == "text" {
			parts = nil
			for _, line := range strings.SplitAfter(segment.Text, "\n") {
				if line != "" {
					parts = append(parts, api.Segment{Type: "text", Text: line})
				}
			}
		}
		for _, part := range parts {
			if atLineStart || part.Type == "block" {
				quoted = part.Type == "text" && strings.HasPrefix(part.Text, ">")
			}
			content := part
			if atLineStart && quoted && part.Type == "text" {
				content.Text = strings.TrimPrefix(strings.TrimPrefix(part.Text, ">"), " ")
			}
			if n := len(blocks); n > 0 && blocks[n-1].quoted == quoted {
				blocks[n-1].segments = append(blocks[n-1].segments, content)
			} else {
				if n > 0 {
					last := &blocks[n-1].segments[len(blocks[n-1].segments)-1]
					if last.Type == "text" {
						last.Text = strings.TrimSuffix(strings.TrimSuffix(last.Text, "\n"), "\r")
					}
				}
				blocks = append(blocks, quoteBlock{quoted: quoted, segments: []api.Segment{content}})
			}
			atLineStart = part.Type == "block" || (part.Type == "text" && strings.HasSuffix(part.Text, "\n"))
		}
	}
	return blocks
}

func (r bodyRenderer) segmentPieces(segments []api.Segment) []piece {
	var pieces []piece
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			pieces = append(pieces, r.linkified(emojify(remoteBody(segment.Text)))...)
		case "code":
			pieces = append(pieces, piece{text: "`" + remoteBody(segment.Code) + "`", paint: text(r.p.code).Render})
		}
	}
	return pieces
}

// wrapPieces breaks pieces into lines at spaces, and inside a word only when the word is wider than the line.
func wrapPieces(pieces []piece, width int) []string {
	var lines []string
	var line strings.Builder
	used, softBreak := 0, false
	var spaces string
	var word []piece
	emit := func(soft bool) {
		lines = append(lines, line.String())
		line.Reset()
		used, spaces, softBreak = 0, "", soft
	}
	place := func() {
		if len(word) == 0 {
			return
		}
		wordWidth := 0
		for _, part := range word {
			wordWidth += ansi.StringWidth(part.text + part.atom)
		}
		if width > 0 && used+ansi.StringWidth(spaces)+wordWidth > width && used > 0 {
			emit(true)
		}
		if !softBreak || used > 0 {
			line.WriteString(spaces)
			used += ansi.StringWidth(spaces)
		}
		spaces = ""
		for _, part := range word {
			if part.atom != "" {
				if width > 0 && used > 0 && used+ansi.StringWidth(part.atom) > width {
					emit(true)
				}
				line.WriteString(part.atom)
				used += ansi.StringWidth(part.atom)
				continue
			}
			rest := part.text
			for rest != "" {
				chunk := rest
				if width > 0 && used+ansi.StringWidth(rest) > width {
					chunk = ansi.Truncate(rest, width-used, "")
					if chunk == "" && used > 0 {
						emit(true)
						continue
					}
					if chunk == "" {
						_, size := utf8.DecodeRuneInString(rest)
						chunk = rest[:size]
					}
				}
				line.WriteString(part.draw(chunk))
				used += ansi.StringWidth(chunk)
				rest = rest[len(chunk):]
				if rest != "" {
					emit(true)
				}
			}
		}
		word = nil
	}
	for _, pc := range pieces {
		if pc.atom != "" {
			word = append(word, pc)
			continue
		}
		for rest := pc.text; rest != ""; {
			cut := strings.IndexAny(rest, " \n")
			if cut != 0 {
				if cut < 0 {
					cut = len(rest)
				}
				word = append(word, piece{text: rest[:cut], paint: pc.paint})
				rest = rest[cut:]
				continue
			}
			place()
			if rest[0] == '\n' {
				emit(false)
			} else {
				spaces += " "
			}
			rest = rest[1:]
		}
	}
	place()
	if !softBreak || used > 0 || len(lines) == 0 {
		line.WriteString(spaces)
		emit(false)
	}
	return lines
}

// emojify turns known :shortcodes: into emoji. Names never hold a colon, so each candidate ends at the next one.
func emojify(s string) string {
	var b strings.Builder
	for {
		open := strings.IndexByte(s, ':')
		if open < 0 {
			break
		}
		closing := strings.IndexByte(s[open+1:], ':')
		if closing < 0 {
			break
		}
		closing += open + 1
		if emoji, ok := emojiCatalog[s[open+1:closing]]; ok {
			b.WriteString(s[:open] + emoji)
			s = s[closing+1:]
			continue
		}
		b.WriteString(s[:closing])
		s = s[closing:]
	}
	b.WriteString(s)
	return b.String()
}

func isJSSpace(r rune) bool {
	return (unicode.IsSpace(r) && r != 0x85) || r == 0xFEFF
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func isAlnum(r rune) bool {
	return r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// endsWord reports a rune that a bare domain or a mention cannot follow.
func endsWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune("_./@-", r)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}

func (r bodyRenderer) linkified(s string) []piece {
	colors := findColors(s)
	var pieces []piece
	position := 0
	for _, match := range findURLs(s) {
		linkText := withoutTrailingPunctuation(s[match[0]:match[1]])
		href, ok := hrefFor(linkText)
		if !ok {
			continue
		}
		pieces = append(pieces, r.mentions(s[position:match[0]], position, colors)...)
		pieces = append(pieces, r.link(linkText, href)...)
		position = match[0] + len(linkText)
	}
	return append(pieces, r.mentions(s[position:], position, colors)...)
}

var topLevelDomains = strings.Fields(`com org net edu gov mil info biz io dev app co ai me xyz tech gg tv fm ly to is blog cloud site online design
	studio tools software codes wiki news website club ninja rocks academy guide social digital agency fyi lol live
	uk de fr nl es it pt br ca au jp ch se no dk fi be at ie nz us eu cz sk hu ro bg gr hr si ee lv lt ru ua tr il za
	mx ar kr cn tw hk sg vn th`)

const notURLRunes = `<>\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

var (
	schemeURL  = regexp.MustCompile(`^[Hh][Tt][Tt][Pp][Ss]?://[^` + notURLRunes + `]+`)
	urlPath    = regexp.MustCompile(`^[/?#][^` + notURLRunes + `]*`)
	domainRune = regexp.MustCompile(`^[A-Za-z0-9-]+`)
)

// findURLs returns where each link candidate starts and ends, before trailing punctuation is dropped.
func findURLs(s string) [][2]int {
	var found [][2]int
	for i := 0; i < len(s); {
		if end := urlAt(s, i); end > i {
			found = append(found, [2]int{i, end})
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return found
}

func urlAt(s string, i int) int {
	before := lastRune(s[:i])
	if i == 0 || (!isWordRune(before) && !unicode.Is(unicode.Pc, before)) {
		if match := schemeURL.FindStringIndex(s[i:]); match != nil {
			return i + match[1]
		}
	}
	if i > 0 && endsWord(before) {
		return i
	}
	return i + bareDomainLength(s[i:])
}

// bareDomainLength matches labels and a known top-level domain, trying the most labels first like the web's regex.
func bareDomainLength(s string) int {
	var ends []int
	for at := 0; ; {
		label := domainRune.FindString(s[at:])
		if label == "" || !isAlnum(rune(label[0])) || !isAlnum(rune(label[len(label)-1])) || !strings.HasPrefix(s[at+len(label):], ".") {
			break
		}
		at += len(label) + 1
		ends = append(ends, at)
	}
	for k := len(ends) - 1; k >= 0; k-- {
		rest := s[ends[k]:]
		for _, upper := range []bool{false, true} {
			for _, domain := range topLevelDomains {
				if upper {
					domain = strings.ToUpper(domain)
				}
				end := ends[k] + len(domain)
				if !strings.HasPrefix(rest, domain) || !domainEnds(s[end:]) {
					continue
				}
				return end + len(urlPath.FindString(s[end:]))
			}
		}
	}
	return 0
}

func domainEnds(rest string) bool {
	next := firstRune(rest)
	if rest != "" && (unicode.IsLetter(next) || unicode.IsNumber(next) || strings.ContainsRune("_(-", next)) {
		return false
	}
	return next != '.' || len(rest) < 2 || !isAlnum(rune(rest[1]))
}

func withoutTrailingPunctuation(candidate string) string {
	for {
		previous := candidate
		candidate = strings.TrimRight(candidate, ".,;:!?'\"…”’")
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}} {
			if strings.HasSuffix(candidate, pair[1]) && strings.Count(candidate, pair[0]) < strings.Count(candidate, pair[1]) {
				candidate = candidate[:len(candidate)-1]
			}
		}
		if candidate == previous {
			return candidate
		}
	}
}

var schemePrefix = regexp.MustCompile(`(?i)^https?://`)

func hrefFor(linkText string) (string, bool) {
	href := linkText
	if !schemePrefix.MatchString(href) {
		href = "https://" + href
	}
	if strings.HasSuffix(href, "://") || utf8.RuneCountInString(href) > 2048 || strings.ContainsFunc(href, unicode.IsControl) {
		return "", false
	}
	return href, true
}

var hiddenURLStart = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?`)

// shortenURL shows a URL without its scheme or "www.", with at most 20 characters after the host.
func shortenURL(link string) string {
	hidden := hiddenURLStart.FindString(link)
	rest := link[len(hidden):]
	host := rest
	if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
		host = rest[:cut]
	}
	// "www." stays when nothing but the top-level domain follows it.
	if strings.HasSuffix(strings.ToLower(hidden), "www.") && !strings.Contains(host, ".") {
		hidden = hidden[:len(hidden)-4]
		rest = link[len(hidden):]
		host = "www." + host
	}
	path := []rune(rest[len(host):])
	if len(path) <= 20 {
		if rest == host+"/" {
			return host
		}
		return rest
	}
	kept := string(path[:19])
	if cut := strings.LastIndexByte(kept, '%'); cut >= len(kept)-2 && cut >= 0 && isHex(kept[cut+1:]) {
		kept = kept[:cut]
	}
	return host + kept + "…"
}

func isHex(s string) bool {
	return strings.Trim(strings.ToLower(s), "0123456789abcdef") == ""
}

func linked(href string, style func(...string) string) func(...string) string {
	return func(s ...string) string { return ansi.SetHyperlink(href) + style(s...) + ansi.ResetHyperlink() }
}

var commitLink = regexp.MustCompile(`(?i)^https?://(?:www\.)?dev-null\.sh/@([A-Za-z0-9-]{1,39})/commits/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

func (r bodyRenderer) link(linkText, href string) []piece {
	p := r.p
	if match := commitLink.FindStringSubmatch(linkText); match != nil && validHandle(match[1]) {
		hash := strings.ReplaceAll(strings.ToLower(match[2]), "-", "")
		return []piece{
			{text: "/", paint: linked(href, text(p.lanes[0]).Bold(true).Render)},
			{text: match[1] + "/", paint: linked(href, text(p.muted).Render)},
			{text: hash[len(hash)-7:], paint: linked(href, text(p.blue).Render)},
		}
	}
	if target, ok := forgeLinkTo(href); ok {
		glyph := map[string]string{"GitHub": r.style.glyphs.github, "GitLab": r.style.glyphs.gitlab, "Codeberg": r.style.glyphs.codeberg}[target.forge]
		if glyph == "" {
			glyph = plainIcons.github
		}
		icon := piece{atom: linked(href, text(p.ink).Render)(glyph + " ")}
		if target.repository == "" {
			return []piece{icon, {text: target.owner, paint: linked(href, text(p.blue).Render)}}
		}
		return []piece{icon, {text: target.owner + "/", paint: linked(href, text(p.muted).Render)}, {text: target.repository + target.suffix, paint: linked(href, text(p.blue).Render)}}
	}
	return []piece{{text: shortenURL(linkText), paint: linked(href, text(p.blue).Render)}}
}

// validHandle is a GitHub handle: up to 39 letters, digits or single inner hyphens.
func validHandle(handle string) bool {
	return len(handle) <= 39 && handle != "" && handle[0] != '-' && !strings.HasSuffix(handle, "-") && !strings.Contains(handle, "--") && strings.Trim(handle, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") == ""
}

type forgeTarget struct{ forge, owner, repository, suffix string }

var (
	gitHubHost       = regexp.MustCompile(`(?i)^(?:www\.)?github\.com$`)
	gitHubRepository = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	gitHubNumber     = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
	gitHubSha        = regexp.MustCompile(`(?i)^[0-9a-f]{7,40}$`)
	gitHubComparison = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9._/-]*?)(\.{2,3})([A-Za-z0-9_][A-Za-z0-9._/-]*)$`)
	gitHubTag        = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+-]*(?:/[A-Za-z0-9._+-]+)*$`)
	gitHubLines      = regexp.MustCompile(`^#L([1-9][0-9]*)(?:-L([1-9][0-9]*))?$`)
	gitHubReserved   = setOf(`about account apps blog codespaces collections contact copilot customer-stories dashboard education enterprise
		enterprises events explore features git-guides home issues join login logout marketplace mobile models new
		notifications orgs organizations partners premium-support pricing pulls readme resources search security
		settings signup site solutions sponsors stars team topics trending users watching`)
	gitHubThreads = map[string]string{"issues": "issue", "pull": "pull request", "discussions": "discussion"}
	gitHubAnchors = map[string][]struct {
		pattern *regexp.Regexp
		name    string
	}{
		"issues":      {{regexp.MustCompile(`^#issuecomment-[1-9][0-9]*$`), "comment"}},
		"pull":        {{regexp.MustCompile(`^#(?:issuecomment-|discussion_r)[1-9][0-9]*$`), "comment"}, {regexp.MustCompile(`^#pullrequestreview-[1-9][0-9]*$`), "review"}},
		"discussions": {{regexp.MustCompile(`^#discussioncomment-[1-9][0-9]*$`), "comment"}},
	}
	otherForges = []struct {
		name     string
		pattern  *regexp.Regexp
		marks    map[string]string
		reserved map[string]bool
	}{
		{"GitLab", regexp.MustCompile(`(?i)^https?://(?:www\.)?gitlab\.com/([A-Za-z0-9][A-Za-z0-9._-]{0,99})/([A-Za-z0-9._-]{1,100})(?:/(-/issues|-/merge_requests)/([1-9][0-9]{0,9}))?$`),
			map[string]string{"-/issues": "#", "-/merge_requests": "!"}, setOf("admin dashboard explore groups help projects search users")},
		{"Codeberg", regexp.MustCompile(`(?i)^https?://(?:www\.)?codeberg\.org/([A-Za-z0-9][A-Za-z0-9._-]{0,99})/([A-Za-z0-9._-]{1,100})(?:/(issues|pulls)/([1-9][0-9]{0,9}))?$`),
			map[string]string{"issues": "#", "pulls": "#"}, setOf("admin api assets explore issues milestones notifications org pulls repo user")},
	}
)

func setOf(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

func forgeLinkTo(href string) (forgeTarget, bool) {
	if target, ok := gitHubLinkTo(href); ok {
		return target, true
	}
	for _, forge := range otherForges {
		match := forge.pattern.FindStringSubmatch(href)
		if match == nil || forge.reserved[strings.ToLower(match[1])] {
			continue
		}
		target := forgeTarget{forge: forge.name, owner: match[1], repository: match[2]}
		if match[3] != "" {
			target.suffix = forge.marks[strings.ToLower(match[3])] + match[4]
		}
		return target, true
	}
	return forgeTarget{}, false
}

func gitHubLinkTo(href string) (forgeTarget, bool) {
	u, err := url.Parse(href)
	if err != nil || !gitHubHost.MatchString(u.Hostname()) || u.User != nil || u.RawQuery != "" {
		return forgeTarget{}, false
	}
	if port := u.Port(); port != "" && port != map[string]string{"http": "80", "https": "443"}[strings.ToLower(u.Scheme)] {
		return forgeTarget{}, false
	}
	hash := ""
	if u.Fragment != "" {
		hash = "#" + u.EscapedFragment()
	}
	segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(segments) == 2 && strings.EqualFold(segments[0], "orgs") {
		segments = segments[1:]
	}
	owner := segments[0]
	if !validHandle(owner) || gitHubReserved[strings.ToLower(owner)] {
		return forgeTarget{}, false
	}
	target := forgeTarget{forge: "GitHub", owner: owner}
	if len(segments) == 1 {
		return target, hash == ""
	}
	target.repository = segments[1]
	if !gitHubRepository.MatchString(target.repository) {
		return forgeTarget{}, false
	}
	if len(segments) == 2 {
		return target, hash == ""
	}
	suffix, ok := gitHubItem(segments[2:], hash)
	target.suffix = suffix
	return target, ok
}

func shortRef(ref string) string {
	if gitHubSha.MatchString(ref) {
		return ref[:7]
	}
	return ref
}

func gitHubItem(path []string, hash string) (string, bool) {
	kind, rest := strings.ToLower(path[0]), path[1:]
	if _, ok := gitHubThreads[kind]; ok {
		if len(rest) != 1 || !gitHubNumber.MatchString(rest[0]) {
			return "", false
		}
		if hash == "" {
			return "#" + rest[0], true
		}
		for _, anchor := range gitHubAnchors[kind] {
			if anchor.pattern.MatchString(hash) {
				return "#" + rest[0] + " (" + anchor.name + ")", true
			}
		}
		return "", false
	}
	if (kind == "blob" || kind == "tree") && len(rest) >= 2 && !slices.Contains(rest, "") {
		return gitHubFile(kind, rest[1:], hash)
	}
	if hash != "" {
		return "", false
	}
	if kind == "commit" && len(rest) == 1 && gitHubSha.MatchString(rest[0]) {
		return "@" + shortRef(rest[0]), true
	}
	if kind == "releases" && len(rest) > 1 && rest[0] == "tag" {
		if tag := strings.Join(rest[1:], "/"); gitHubTag.MatchString(tag) {
			return "@" + tag, true
		}
	}
	if kind == "compare" {
		if match := gitHubComparison.FindStringSubmatch(strings.Join(rest, "/")); match != nil && !strings.Contains(match[3], "..") {
			return "@" + shortRef(match[1]) + match[2] + shortRef(match[3]), true
		}
	}
	return "", false
}

// gitHubFile names a file, maybe with its lines, or a folder, by its last name only.
func gitHubFile(kind string, path []string, hash string) (string, bool) {
	name, err := url.PathUnescape(path[len(path)-1])
	if err != nil {
		return "", false
	}
	for _, segment := range path {
		if _, err := url.PathUnescape(segment); err != nil {
			return "", false
		}
	}
	name = safeText(name)
	if kind == "tree" {
		return ":" + name + "/", hash == ""
	}
	if hash == "" {
		return ":" + name, true
	}
	lines := gitHubLines.FindStringSubmatch(hash)
	if lines == nil {
		return "", false
	}
	if lines[2] == "" || lines[2] == lines[1] {
		return ":" + name + ":" + lines[1], true
	}
	return ":" + name + ":" + lines[1] + "-" + lines[2], true
}

func (r bodyRenderer) mentions(s string, offset int, colors []hexColor) []piece {
	var pieces []piece
	position := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '@' || (i > 0 && endsWord(lastRune(s[:i]))) {
			continue
		}
		end := handleEnd(s, i+1)
		if end == i+1 || !mentionEnds(s[end:]) {
			continue
		}
		pieces = append(pieces, r.colored(s[position:i], offset+position, colors)...)
		pieces = append(pieces, piece{text: s[i:end], paint: r.profileLink(s[i+1 : end])})
		position = end
		i = end - 1
	}
	return append(pieces, r.colored(s[position:], offset+position, colors)...)
}

// profileLink styles a mention, linked to the profile on the web when the server is known. The web's route also
// serves organizations under the same path.
func (r bodyRenderer) profileLink(handle string) func(...string) string {
	style := text(r.p.blue).Bold(true).Render
	origin, err := url.Parse(r.style.origin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" {
		return style
	}
	href, ok := hrefFor(strings.TrimRight(r.style.origin, "/") + "/@" + handle)
	if !ok {
		return style
	}
	return linked(href, style)
}

// handleEnd reads the longest GitHub handle at start: up to 39 characters, a hyphen only before a letter or digit.
func handleEnd(s string, start int) int {
	if start >= len(s) || !isAlnum(rune(s[start])) {
		return start
	}
	end := start + 1
	for end-start < 39 && end < len(s) {
		switch {
		case isAlnum(rune(s[end])):
			end++
		case s[end] == '-' && end+1 < len(s) && isAlnum(rune(s[end+1])):
			end++
		default:
			return end
		}
	}
	return end
}

func mentionEnds(rest string) bool {
	if rest == "" {
		return true
	}
	if isAlnum(rune(rest[0])) || strings.ContainsRune("_@/-", rune(rest[0])) {
		return false
	}
	return rest[0] != '.' || len(rest) < 2 || !isAlnum(rune(rest[1]))
}

type hexColor struct {
	start, end int
	value      string
}

// findColors finds hex colors that stand as whole words, like "#ff8a4c" or "#fff". Three or four digits need a letter
// or a leading zero, so "#123" stays an issue reference.
func findColors(s string) []hexColor {
	var colors []hexColor
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		before := lastRune(s[:i])
		if (r != '#' && r != '\uff03') || (i > 0 && !isJSSpace(before) && !strings.HasSuffix(s[:i], "\n>")) {
			i += size
			continue
		}
		digits := s[i+size:]
		run := len(digits) - len(strings.TrimLeft(digits, "0123456789abcdefABCDEF"))
		found := false
		for _, length := range []int{8, 6, 4, 3} {
			if run < length || (length < 6 && !shortColorAllowed(digits)) || !colorEnds(digits[length:]) {
				continue
			}
			colors = append(colors, hexColor{start: i, end: i + size + length, value: strings.ToLower(digits[:length])})
			i += size + length
			found = true
			break
		}
		if !found {
			i += size
		}
	}
	return colors
}

func shortColorAllowed(digits string) bool {
	if strings.HasPrefix(digits, "0") {
		return true
	}
	rest := strings.TrimLeft(digits, "0123456789")
	return rest != "" && strings.ContainsRune("abcdefABCDEF", rune(rest[0]))
}

func colorEnds(rest string) bool {
	rest = strings.TrimLeftFunc(rest, func(r rune) bool { return unicode.IsPunct(r) })
	next := firstRune(rest)
	return rest == "" || isJSSpace(next) || next == '\u00ad' || next == '\u2060' || (next >= '\u200a' && next <= '\u200d') || next == '\u20e2'
}

func swatchColor(value string) string {
	if len(value) >= 6 {
		return "#" + value[:6]
	}
	return "#" + string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
}

// swatch fills a square with the color, or outlines it when the color would vanish into the background. The web
// draws a border around every swatch, which a terminal cell cannot.
func (r bodyRenderer) swatch(color string) string {
	if contrast(color, r.p.shell) < 1.25 {
		return text(r.p.muted).Render("□")
	}
	return text(color).Render("■")
}

// contrast is the WCAG contrast ratio between two "#rrggbb" colors.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func luminance(hex string) float64 {
	value, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return 0
	}
	channel := func(shift uint) float64 {
		c := float64((value>>shift)&255) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(16) + 0.7152*channel(8) + 0.0722*channel(0)
}

func (r bodyRenderer) colored(s string, offset int, colors []hexColor) []piece {
	var pieces []piece
	position := 0
	for _, color := range colors {
		start, end := color.start-offset, color.end-offset
		if start < position || end > len(s) {
			continue
		}
		pieces = append(pieces, r.wordmarks(s[position:start])...)
		pieces = append(pieces, piece{atom: r.swatch(swatchColor(color.value)) + " "}, piece{text: s[start:end]})
		position = end
	}
	return append(pieces, r.wordmarks(s[position:])...)
}

const wordmark = "/dev/null"

func (r bodyRenderer) wordmarks(s string) []piece {
	var pieces []piece
	position := 0
	for search := 0; ; {
		found := strings.Index(s[search:], wordmark)
		if found < 0 {
			break
		}
		start, end := search+found, search+found+len(wordmark)
		search = start + 1
		before, after := lastRune(s[:start]), firstRune(s[end:])
		if (start > 0 && (isWordRune(before) || strings.ContainsRune("./-", before))) || (end < len(s) && (isWordRune(after) || strings.ContainsRune("/-", after))) {
			continue
		}
		bold := text(r.p.ink).Bold(true).Render
		pieces = append(pieces, r.pasteis(s[position:start])...)
		pieces = append(pieces, piece{text: "/dev", paint: bold}, piece{text: "/", paint: text(r.p.lanes[0]).Bold(true).Render}, piece{text: "null", paint: bold})
		position, search = end, end
	}
	return append(pieces, r.pasteis(s[position:])...)
}

var pastelWords = []string{"pastel", "pasteis", "pastéis"}

// pasteis swaps the words for the pastel icon in pastéis mode, or colors them until the icon is ready.
func (r bodyRenderer) pasteis(s string) []piece {
	if !r.style.pasteis {
		return []piece{{text: s}}
	}
	var pieces []piece
	position := 0
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		if i > 0 && isWordRune(lastRune(s[:i])) {
			i += size
			continue
		}
		word := ""
		for _, candidate := range pastelWords {
			end := i
			for range utf8.RuneCountInString(candidate) {
				if end >= len(s) {
					break
				}
				_, n := utf8.DecodeRuneInString(s[end:])
				end += n
			}
			if strings.EqualFold(s[i:end], candidate) && (end == len(s) || !isWordRune(firstRune(s[end:]))) {
				word = s[i:end]
				break
			}
		}
		if word == "" {
			i += size
			continue
		}
		pieces = append(pieces, piece{text: s[position:i]})
		if icon := r.pastelIcon(); icon != "" {
			pieces = append(pieces, piece{atom: icon})
		} else {
			pieces = append(pieces, piece{text: word, paint: text(r.p.lanes[0]).Render})
		}
		i += len(word)
		position = i
	}
	return append(pieces, piece{text: s[position:]})
}

func (r bodyRenderer) pastelIcon() string {
	if r.style.icons == nil {
		return ""
	}
	return r.style.icons.Row(pastelIconRequest, 0)
}

var codeMarkup = regexp.MustCompile(`<span class="([^"]*)">|</span>|<[^>]*>`)

// codeBlock draws highlighted code under its filename or language, colored like the web's hl-* classes.
func (r bodyRenderer) codeBlock(segment api.Segment, width int) []string {
	p := r.p
	colors := map[string]string{
		"hl-keyword": p.blue, "hl-type": p.brand, "hl-generic": p.brand, "hl-property": p.lanes[3], "hl-value": p.lanes[1],
		"hl-number": p.lanes[0], "hl-literal": p.lanes[0], "hl-variable": p.lanes[4], "hl-comment": p.muted,
		"hl-addition": p.lanes[1], "hl-deletion": p.danger,
	}
	var stack [][]string
	var pieces []piece
	write := func(raw string) {
		if raw == "" {
			return
		}
		color, italic := p.ink, false
		for _, classes := range stack {
			for _, class := range classes {
				if mapped, ok := colors[class]; ok {
					color = mapped
				}
				italic = italic || class == "hl-comment" || class == "hl-attribute"
			}
		}
		pieces = append(pieces, piece{text: remoteBody(html.UnescapeString(raw)), paint: text(color).Italic(italic).Render})
	}
	position := 0
	for _, match := range codeMarkup.FindAllStringSubmatchIndex(segment.HTML, -1) {
		write(segment.HTML[position:match[0]])
		position = match[1]
		switch tag := segment.HTML[match[0]:match[1]]; {
		case match[2] >= 0:
			stack = append(stack, strings.Fields(segment.HTML[match[2]:match[3]]))
		case tag == "</span>" && len(stack) > 0:
			stack = stack[:len(stack)-1]
		}
	}
	write(segment.HTML[position:])
	var lines []string
	header := safeText(segment.Filename)
	if header == "" {
		header = safeText(segment.Language)
	}
	if header != "" {
		lines = append(lines, text(p.muted).Render(header))
	}
	code := wrapPieces(pieces, width)
	for len(code) > 0 && ansi.Strip(code[len(code)-1]) == "" {
		code = code[:len(code)-1]
	}
	return append(lines, prefixLines(code, text(p.hairline).Render("│ "))...)
}
