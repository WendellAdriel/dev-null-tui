package api

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBodyLength = 4000
const MaxRawBodyLength = 40000

var codeSpans = regexp.MustCompile("(?m)^`{3,}[^`\r\n]*\n(?:[\\s\\S]*?\n)?`{3,}[ \\t]*$|`[^`\r\n]+`")
var blankLines = regexp.MustCompile(`(?:[ \t]*\n){3,}`)
var urlCandidates = regexp.MustCompile(`[Hh][Tt][Tt][Pp][Ss]?://[^\s\p{Z}\x{000B}\x{0085}<>]+|(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+[A-Za-z]+(?:[/?#][^\s\p{Z}\x{000B}\x{0085}<>]*)?`)

// Match Laravel Str::INVISIBLE_CHARACTERS at the signed API boundary.
const invisibleTrim = "\u00ad\u034f\u061c\u115f\u1160\u17b4\u17b5\u180e\u200b\u200c\u200d\u200e\u200f\u2060\u2061\u2062\u2063\u2064\u2065\u206a\u206b\u206c\u206d\u206e\u206f\u2800\u3164\ufeff\uffa0\U0001d159\U0001d173\U0001d174\U0001d175\U0001d176\U0001d177\U0001d178\U0001d179\U0001d17a\U000e0020"

const bareTLDs = "com org net edu gov mil info biz io dev app co ai me xyz tech gg tv fm ly to is blog cloud site online design studio tools software codes wiki news website club ninja rocks academy guide social digital agency fyi lol live uk de fr nl es it pt br ca au jp ch se no dk fi be at ie nz us eu cz sk hu ro bg gr hr si ee lv lt ru ua tr il za mx ar kr cn tw hk sg vn th"

func mapProse(body string, fn func(string) string) string {
	var out strings.Builder
	start := 0
	for _, span := range codeSpans.FindAllStringIndex(body, -1) {
		out.WriteString(fn(body[start:span[0]]))
		out.WriteString(body[span[0]:span[1]])
		start = span[1]
	}
	out.WriteString(fn(body[start:]))
	return out.String()
}

func NormalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\x00", "")
	body = strings.TrimFunc(body, func(r rune) bool { return r == 0 || unicode.IsSpace(r) || strings.ContainsRune(invisibleTrim, r) })
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	return mapProse(body, func(prose string) string { return blankLines.ReplaceAllString(prose, "\n\n") })
}

func proseURLs(prose string) []string {
	var urls []string
	for _, span := range urlCandidates.FindAllStringIndex(prose, -1) {
		candidate := prose[span[0]:span[1]]
		scheme := strings.HasPrefix(strings.ToLower(candidate), "http://") || strings.HasPrefix(strings.ToLower(candidate), "https://")
		if span[0] > 0 {
			previous, _ := utf8.DecodeLastRuneInString(prose[:span[0]])
			blocked := unicode.IsLetter(previous) || unicode.IsNumber(previous)
			if scheme {
				blocked = blocked || unicode.IsMark(previous) || unicode.In(previous, unicode.Pc)
			} else {
				blocked = blocked || strings.ContainsRune("_./@-", previous)
			}
			if blocked {
				continue
			}
		}
		if !scheme {
			host := strings.FieldsFunc(candidate, func(r rune) bool { return r == '/' || r == '?' || r == '#' })[0]
			tld := host[strings.LastIndex(host, ".")+1:]
			if tld != strings.ToLower(tld) && tld != strings.ToUpper(tld) {
				continue
			}
			if !strings.Contains(" "+bareTLDs+" ", " "+strings.ToLower(tld)+" ") {
				continue
			}
			if span[1] < len(prose) {
				next, _ := utf8.DecodeRuneInString(prose[span[1]:])
				if unicode.IsLetter(next) || unicode.IsNumber(next) || strings.ContainsRune("_(-", next) {
					continue
				}
				if next == '.' && span[1]+1 < len(prose) {
					after := prose[span[1]+1]
					if (after >= 'A' && after <= 'Z') || (after >= 'a' && after <= 'z') || (after >= '0' && after <= '9') {
						continue
					}
				}
			}
		}
		for {
			previous := candidate
			candidate = strings.TrimRight(candidate, ".,;:!?'\"…”’")
			for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}} {
				if strings.HasSuffix(candidate, pair[1]) && strings.Count(candidate, pair[0]) < strings.Count(candidate, pair[1]) {
					candidate = strings.TrimSuffix(candidate, pair[1])
				}
			}
			if previous == candidate {
				break
			}
		}
		if !strings.HasSuffix(candidate, "://") {
			urls = append(urls, candidate)
		}
	}
	return urls
}

func bodyBudget(body string) (int, bool) {
	count, oversized := 0, false
	mapProse(NormalizeBody(body), func(prose string) string {
		count += utf8.RuneCountInString(prose)
		for _, link := range proseURLs(prose) {
			length := utf8.RuneCountInString(link)
			urlLength := length
			if !strings.HasPrefix(strings.ToLower(link), "http://") && !strings.HasPrefix(strings.ToLower(link), "https://") {
				urlLength += 8
			}
			if urlLength > 2048 {
				oversized = true
				continue
			}
			count -= max(0, length-23)
		}
		return ""
	})
	for _, span := range codeSpans.FindAllString(NormalizeBody(body), -1) {
		count += utf8.RuneCountInString(span)
	}
	return count, oversized
}

func BodyCharacterCount(body string) int { count, _ := bodyBudget(body); return count }

func ValidateBody(body string) error {
	if NormalizeBody(body) == "" {
		return errors.New("write a commit first")
	}
	if utf8.RuneCountInString(NormalizeBody(body)) > MaxRawBodyLength {
		return errors.New("raw text exceeds 40000 characters")
	}
	count, oversized := bodyBudget(body)
	if oversized {
		return errors.New("each link must fit within 2048 characters")
	}
	if count > MaxBodyLength {
		return errors.New("commit exceeds the 4000 character budget")
	}
	return nil
}
