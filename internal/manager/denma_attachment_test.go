package manager

import (
	"mime"
	"testing"
)

// An attachment's name with a space, accent or quote comes through whole in
// both headers, and a content type's own parameters are kept.
func TestDenmaAttachmentName(t *testing.T) {
	for _, name := range []string{"Spring Newsletter.pdf", "Été 2026.pdf", `The "Quote".pdf`, "plain.pdf"} {
		h := MakeAttachmentHeader(name, "base64", "application/pdf")

		disp, dp, err := mime.ParseMediaType(h.Get("Content-Disposition"))
		if err != nil || disp != "attachment" || dp["filename"] != name {
			t.Errorf("%q: Content-Disposition %q gives %q, %v", name, h.Get("Content-Disposition"), dp["filename"], err)
		}
		typ, tp, err := mime.ParseMediaType(h.Get("Content-Type"))
		if err != nil || typ != "application/pdf" || tp["name"] != name {
			t.Errorf("%q: Content-Type %q gives %q, %v", name, h.Get("Content-Type"), tp["name"], err)
		}
	}

	h := MakeAttachmentHeader("notes.txt", "", "text/plain; charset=utf-8")
	if _, p, _ := mime.ParseMediaType(h.Get("Content-Type")); p["charset"] != "utf-8" || p["name"] != "notes.txt" {
		t.Errorf("charset not kept: %q", h.Get("Content-Type"))
	}
	for _, ct := range []string{"", "nonsense"} {
		h := MakeAttachmentHeader("a.bin", "", ct)
		if typ, _, _ := mime.ParseMediaType(h.Get("Content-Type")); typ != "application/octet-stream" {
			t.Errorf("%q: Content-Type %q", ct, h.Get("Content-Type"))
		}
	}

	h = MakeInlineAttachmentHeader("Logo Été.png", "", "image/png", "c1")
	if disp, p, _ := mime.ParseMediaType(h.Get("Content-Disposition")); disp != "inline" || p["filename"] != "Logo Été.png" || h.Get("Content-ID") != "<c1>" {
		t.Errorf("inline: %v", h)
	}
}
