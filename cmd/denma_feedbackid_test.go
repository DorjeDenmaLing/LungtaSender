package main

import (
	"net/textproto"
	"testing"

	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/models"
)

// Each e-mail's Feedback-ID fields name its campaign (or kind) and its center,
// in the characters SES takes.
func TestDenmaFeedbackTag(t *testing.T) {
	camp := &models.Campaign{UUID: "5b3c-41d2-9e0f"}
	camp.ID = 3
	auto := models.Message{Headers: textproto.MIMEHeader{}}
	denmaSentAs(&auto, 7, 2)
	test := textproto.MIMEHeader{}
	test.Set(manager.DenmaCampaignTest, "1")
	for _, c := range []struct {
		m      models.Message
		center string
		want   string
	}{
		{models.Message{Campaign: camp}, "halifax", "ses:feedback-id-a=5b3c-41d2-9e0f, ses:feedback-id-b=halifax"},
		{models.Message{Campaign: camp, Headers: test}, "halifax", "ses:feedback-id-a=test, ses:feedback-id-b=halifax"},
		{auto, "ddl", "ses:feedback-id-a=automation, ses:feedback-id-b=ddl"},
		{models.Message{Subject: "Confirm"}, denmaFeedbackHub, "ses:feedback-id-a=transactional, ses:feedback-id-b=hub"},
		{models.Message{Campaign: &models.Campaign{Base: models.Base{ID: 4}}}, "a b,c=d", "ses:feedback-id-a=campaign, ses:feedback-id-b=abcd"},
	} {
		if got := denmaFeedbackTag(c.m, c.center); got != c.want {
			t.Errorf("%q: %q, want %q", c.center, got, c.want)
		}
	}
	if got := denmaTagValue(":,="); got != "none" {
		t.Errorf("empty value: %q", got)
	}
}
