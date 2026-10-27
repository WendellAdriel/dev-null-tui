package api

import "testing"

func TestNormalizeBodyRemovesEmbeddedNulls(t *testing.T) {
	for _, body := range []string{" before\x00after ", "before\x00\x00after", "\x00beforeafter\x00"} {
		if got := NormalizeBody(body); got != "beforeafter" {
			t.Fatalf("normalized body = %q", got)
		}
	}
	if got := NormalizeBody("`a\x00b`\n\n```go\nx\x00y\n```"); got != "`ab`\n\n```go\nxy\n```" {
		t.Fatalf("normalized code = %q", got)
	}
	if BodyCharacterCount("a\x00b") != 2 || ValidateBody("\x00\x00") == nil {
		t.Fatal("body validation must use the server's NUL-free text")
	}
}
