package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// The web's mention-textarea.tsx rules. Keep the mention pattern in step with the server's app/Services/Mentions.php.
const suggestionDebounce = 120 * time.Millisecond

var (
	mentionBeforeCaret = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_./@-])@([A-Za-z0-9][A-Za-z0-9-]{0,38}|)$`)
	issueBeforeCaret   = regexp.MustCompile(`(?:^|[\s(\[{])#([1-9][0-9]{0,9})$`)
)

// completionCalls are the composer suggestion routes, which tests replace.
type completionCalls struct {
	mentions func(context.Context, api.MentionQuery) (api.Profiles, error)
	issue    func(context.Context, string) (*api.Issue, error)
}

func newCompletionCalls(client *api.Client) completionCalls {
	return completionCalls{client.MentionSuggestions, client.IssueSuggestion}
}

type suggestedProfile struct {
	handle, name string
	lane         json.RawMessage
	organization bool
}

// completion is an editor's suggestions for the word before the caret: emoji for a :shortcode, people and
// organizations for an @mention, or the issue a #number names. key is the lookup the shown results must answer.
type completion struct {
	kind, query, prefix, dismissed string
	emojis                         []emojiSuggestion
	people                         []suggestedProfile
	issue                          *api.Issue
	index                          int
	key, found                     string
	generation                     int
	cancel                         context.CancelFunc
	cache                          map[string][]suggestedProfile
}

type completionTick struct {
	session, generation int
	owner               string
}

type completionResult struct {
	session, generation int
	owner, key          string
	people              []suggestedProfile
	issue               *api.Issue
	err                 error
}

func (c *completion) count() int {
	switch {
	case c.kind == "emoji":
		return len(c.emojis)
	case c.key != c.found:
		return 0
	case c.kind == "mention":
		return len(c.people)
	case c.kind == "issue" && c.issue != nil:
		return 1
	}
	return 0
}

// close hides the suggestions and drops any lookup still waiting on its debounce.
func (c *completion) close() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.kind, c.emojis, c.key = "", nil, ""
	c.generation = 0
}

// dismiss hides the suggestions until the text before the caret changes, like Esc on the web.
func (c *completion) dismiss() {
	c.dismissed = c.prefix
	c.close()
}

func (c *completion) move(step int) {
	if n := c.count(); n > 0 {
		c.index = (c.index + step + n) % n
	}
}

// labels are the suggestions as plain text, in order.
func (c *completion) labels() []string {
	var labels []string
	switch c.kind {
	case "emoji":
		for _, e := range c.emojis {
			labels = append(labels, e.value+" :"+e.name+":")
		}
	case "mention":
		for _, person := range c.people[:c.count()] {
			label := "@" + safeText(person.handle)
			if name := safeText(person.name); name != "" {
				label += " " + name
			}
			if person.organization {
				label += " · org"
			}
			labels = append(labels, label)
		}
	case "issue":
		if c.count() == 1 {
			labels = append(labels, "#"+c.query+" "+strings.Join(strings.Fields(safeText(c.issue.Title)), " "))
		}
	}
	return labels
}

// completionTrigger finds what the text before the caret is asking for, in the web's order: a mention, then an emoji
// shortcode in prose, then an issue number that isn't a color.
func completionTrigger(value, before string) (kind, query string) {
	if match := mentionBeforeCaret.FindStringSubmatch(before); match != nil {
		return "mention", match[1]
	}
	if query, closed := emojiAtCaret(before); query != "" && !closed {
		return "emoji", query
	}
	prose, ok := proseBeforeCaret(before)
	match := issueBeforeCaret.FindStringSubmatch(prose)
	if !ok || match == nil {
		return "", ""
	}
	if next, _ := utf8.DecodeRuneInString(value[len(before):]); unicode.IsLetter(next) || unicode.IsNumber(next) || unicode.Is(unicode.M, next) || strings.ContainsRune("_/#-", next) {
		return "", ""
	}
	start := len(before) - len(match[1]) - 1
	for _, color := range findColors(value) {
		if color.start == start {
			return "", ""
		}
	}
	return "issue", match[1]
}

// proseBeforeCaret is the text after the last code span, or false while the caret is inside one.
func proseBeforeCaret(before string) (string, bool) {
	start := 0
	for _, span := range typedCode.FindAllStringIndex(before, -1) {
		if span[1] == len(before) {
			return "", false
		}
		start = span[1]
	}
	return before[start:], true
}

func mentionKey(scope api.MentionQuery, query string) string {
	return scope.Org + ":" + scope.Reply + ":" + scope.Quote + ":" + strings.ToLower(query)
}

// refreshCompletion updates the suggestions after the caret or the text moved, and starts a lookup when one is due.
func (m *AppModel) refreshCompletion(c *completion, input textarea.Model, owner string, scope api.MentionQuery) tea.Cmd {
	prefix := textBeforeCaret(input)
	kind, query := completionTrigger(input.Value(), prefix)
	if kind == "" || prefix == c.dismissed || input.HasSelection() || (kind != "emoji" && m.completionCalls.mentions == nil) {
		c.close()
		return nil
	}
	if prefix != c.prefix || kind != c.kind {
		c.index = 0
	}
	c.kind, c.query, c.prefix = kind, query, prefix
	switch kind {
	case "emoji":
		c.emojis, c.generation = searchEmoji(query), 0
		return nil
	case "mention":
		key := mentionKey(scope, query)
		if cached, ok := c.cache[key]; ok {
			c.key, c.found, c.people, c.generation = key, key, cached, 0
			return nil
		}
		delay := suggestionDebounce
		if query == "" {
			delay = 0
		}
		return m.startLookup(c, owner, key, delay)
	}
	return m.startLookup(c, owner, "#"+query, suggestionDebounce)
}

func (m *AppModel) startLookup(c *completion, owner, key string, delay time.Duration) tea.Cmd {
	if key == c.key {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	m.completionGeneration++
	c.key, c.generation = key, m.completionGeneration
	session, generation := m.session, c.generation
	return tea.Tick(delay, func(time.Time) tea.Msg { return completionTick{session, generation, owner} })
}

// completionOf is the editor a lookup belongs to, while it is still open.
func (m *AppModel) completionOf(owner string) (*completion, api.MentionQuery) {
	switch owner {
	case "commit":
		if m.composer.open {
			return &m.composer.suggest, m.composerMentionScope()
		}
	case "message":
		if t := m.remotes.thread; t != nil {
			return &t.composer.suggest, m.messageMentionScope()
		}
	}
	return nil, api.MentionQuery{}
}

func (m *AppModel) acceptCompletionTick(msg completionTick) tea.Cmd {
	c, scope := m.completionOf(msg.owner)
	if c == nil || msg.session != m.session || msg.generation != c.generation || (c.kind != "mention" && c.kind != "issue") {
		return nil
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	c.cancel = cancel
	result := completionResult{session: m.session, generation: msg.generation, owner: msg.owner, key: c.key}
	calls, kind, query := m.completionCalls, c.kind, c.query
	scope.Term = query
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		if kind == "issue" {
			result.issue, result.err = calls.issue(ctx, query)
			return result
		}
		found, err := calls.mentions(ctx, scope)
		result.err = err
		for _, user := range found.Users {
			result.people = append(result.people, suggestedProfile{handle: user.Handle, name: user.Name, lane: user.Lane})
		}
		for _, organization := range found.Organizations {
			result.people = append(result.people, suggestedProfile{handle: organization.Handle, name: organization.Name, lane: organization.Lane, organization: true})
		}
		return result
	})
}

func (m *AppModel) acceptCompletionResult(msg completionResult) tea.Cmd {
	c, scope := m.completionOf(msg.owner)
	if c == nil || msg.session != m.session || msg.generation != c.generation {
		return nil
	}
	c.cancel = nil
	if msg.err != nil {
		var failure *auth.APIError
		if m.revoked(msg.err) || !errors.As(msg.err, &failure) || failure.Status != http.StatusTooManyRequests || c.kind != "mention" {
			return nil
		}
		return m.notify("warning", "Slow down", "Too many suggestions. Give it a few seconds.")
	}
	c.found, c.people, c.issue = msg.key, msg.people, msg.issue
	// Replies need fresh participants when someone joins the thread, so they are never cached.
	if c.kind == "mention" && scope.Reply == "" {
		if c.cache == nil {
			c.cache = make(map[string][]suggestedProfile)
		}
		c.cache[msg.key] = msg.people
	}
	m.reflow()
	return nil
}

// acceptCompletion swaps the word before the caret for the chosen suggestion.
func acceptCompletion(c *completion, input *textarea.Model) {
	if c.count() == 0 {
		return
	}
	index := min(c.index, c.count()-1)
	if c.kind == "emoji" {
		replaceShortcode(input, c.query, c.emojis[index].value, false)
		c.close()
		return
	}
	var insert string
	if c.kind == "mention" {
		insert = "@" + c.people[index].handle
	} else {
		insert = c.issue.URL
	}
	after := input.Value()[len(textBeforeCaret(*input)):]
	for range utf8.RuneCountInString(c.query) + 1 {
		*input, _ = input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	input.InsertString(insert)
	if next, _ := utf8.DecodeRuneInString(after); !unicode.IsSpace(next) {
		input.InsertString(" ")
	}
	c.close()
}

// composerMentionScope suggests a reply's or quote's thread participants first, as the web does.
func (m AppModel) composerMentionScope() api.MentionQuery {
	switch c := m.composer; c.mode {
	case "reply":
		return api.MentionQuery{Reply: c.target.ID}
	case "quote":
		return api.MentionQuery{Quote: c.target.ID}
	}
	return api.MentionQuery{}
}

// messageMentionScope narrows an org's suggestions to its members.
func (m AppModel) messageMentionScope() api.MentionQuery {
	if t := m.remotes.thread; t != nil && t.conversation.Kind == "org" {
		return api.MentionQuery{Org: t.conversation.ID}
	}
	return api.MentionQuery{}
}
