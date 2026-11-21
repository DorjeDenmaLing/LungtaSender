package main

import (
	"strings"
	"testing"
)

// A saved design's logo with a fixed height gets a maximum height, as the builder now writes it;
// an image with a width, and a design already fixed, are left as they are.
func TestDenmaImgHeightsHTML(t *testing.T) {
	body, _ := denmaDefaultDesign("https://example.org/logo.png", "Halifax")
	old := strings.Replace(body, `style="height:auto;max-height:90px;`, `style="height:90px;`, 1)
	if old == body {
		t.Fatal("the default design's logo has no maximum height")
	}
	if got := denmaImgHeightsHTML(old); got != body {
		t.Errorf("an old design's logo wasn't fixed:\n%s", got)
	}
	if got := denmaImgHeightsHTML(body); got != body {
		t.Error("a fixed design was changed")
	}
	sized := `<img alt="x" src="https://example.org/a.png" width="200" height="90" style="width:200px;height:90px;outline:none;border:none"/>`
	if got := denmaImgHeightsHTML(sized); got != sized {
		t.Errorf("an image with a width was changed: %s", got)
	}
}

// A design with listmonk's logo gets the copy at the design's logo height, in its HTML and source alike.
func TestDenmaStockLogoHTML(t *testing.T) {
	body, src := denmaDefaultDesign(denmaDesignLogo("", "http://localhost:9000/c/halifax"), "Halifax")
	oldBody := strings.ReplaceAll(body, denmaDesignStockLogo, "logo.svg")
	oldSrc := strings.ReplaceAll(src, denmaDesignStockLogo, "logo.svg")
	if oldBody == body || oldSrc == src {
		t.Fatal("the default design doesn't use the stock logo")
	}
	if denmaStockLogoHTML(oldBody) != body || denmaStockLogoHTML(oldSrc) != src {
		t.Error("an old design's stock logo wasn't replaced")
	}
	own := `<img alt="x" src="https://example.org/public/static/logo.svg?v=2" height="90"/>`
	if denmaStockLogoHTML(own) != own {
		t.Error("a logo URL with a query was changed")
	}
}
