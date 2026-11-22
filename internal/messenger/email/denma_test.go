package email

import (
	"net/textproto"
	"testing"

	"github.com/knadh/listmonk/models"
)

// A message's own header is refused (and left out of the e-mail) if it'd
// change the sender, add recipients, set an internal header, or carry a line
// break that would start a header of its own.
func TestDenmaHeaderProblem(t *testing.T) {
	for _, c := range []struct{ name, value, want string }{
		{"Reply-To", "office@example.org", ""},
		{"X-Campaign", "spring\tretreat", ""},
		{"List-Unsubscribe", "<https://example.org/u>", ""},
		{"from", "x@example.org", "sender"},
		{"X-SES-CONFIGURATION-SET", "other", "sender"},
		{"Bcc", "other@example.org", "recipients"},
		{"cc", "other@example.org", "recipients"},
		{"TO", "other@example.org", "recipients"},
		{"X-Denma-Daily-Claim", "1", "internal"},
		{"x-denma-center-claim", "1", "internal"},
		{"X-A", "b\r\nSender: news@other.org", "control"},
		{"Content-Disposition", "inline\nX-SES-SOURCE-ARN: x", "control"},
		{"X-A", "b\x00", "control"},
		{"X-A: b\r\nSender", "news@other.org", "name"},
		{" From", "x@example.org", "name"},
		{"X A", "b", "name"},
		{"X-Ä", "b", "name"},
		{"", "b", "name"},
	} {
		if got := DenmaHeaderProblem(c.name, c.value); got != c.want {
			t.Errorf("%q: %q = %q, want %q", c.name, c.value, got, c.want)
		}
	}
	if !denmaDropHeader("X-A", []string{"ok", "b\r\nBcc: x@example.org"}) {
		t.Error("a bad second value wasn't dropped")
	}
	if denmaDropHeader("X-A", []string{"ok"}) {
		t.Error("a good header was dropped")
	}
}

// An e-mail's own SES tags come after those of the server's headers.
func TestDenmaTag(t *testing.T) {
	e := &Emailer{}
	h := textproto.MIMEHeader{}
	e.denmaTag(h, models.Message{})
	if len(h) != 0 {
		t.Fatalf("no tagger: %v", h)
	}
	e.DenmaTagWith(func(models.Message) string { return "ses:feedback-id-a=x" })
	e.denmaTag(h, models.Message{})
	if got := h.Get("X-SES-MESSAGE-TAGS"); got != "ses:feedback-id-a=x" {
		t.Errorf("alone: %q", got)
	}
	h = textproto.MIMEHeader{}
	h.Set("X-SES-MESSAGE-TAGS", "team=news")
	e.denmaTag(h, models.Message{})
	if got := h.Get("X-SES-MESSAGE-TAGS"); got != "team=news, ses:feedback-id-a=x" {
		t.Errorf("after the server's: %q", got)
	}
}
