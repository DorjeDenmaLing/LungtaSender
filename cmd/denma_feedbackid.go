package main

// denma: SES message tags that fill the first two fields of Gmail's Feedback-ID: the campaign or kind of e-mail, and the center.

import (
	"strings"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/models"
)

// denmaFeedbackHub is the center field of the hub's own e-mails.
const denmaFeedbackHub = "hub"

// denmaFeedbackTags has an app's e-mail messenger tag each e-mail with its Feedback-ID fields.
func denmaFeedbackTags(em *email.Emailer, ko *koanf.Koanf) {
	if em == nil || !ko.Bool("denma.multi_center") {
		return
	}
	center := ko.String("denma.center")
	if center == "" {
		center = denmaFeedbackHub
	}
	em.DenmaTagWith(func(m models.Message) string { return denmaFeedbackTag(m, center) })
}

// denmaFeedbackTag is an e-mail's tags: ses:feedback-id-a its campaign or kind, ses:feedback-id-b its center.
func denmaFeedbackTag(m models.Message, center string) string {
	return "ses:feedback-id-a=" + denmaTagValue(denmaFeedbackKind(m)) +
		", ses:feedback-id-b=" + denmaTagValue(center)
}

// denmaFeedbackKind is a campaign's UUID, or automation, test or transactional.
func denmaFeedbackKind(m models.Message) string {
	kind, _, _, _ := denmaMessageKind(m)
	switch kind {
	case denmaMsgCampaign:
		if m.Campaign.UUID != "" {
			return m.Campaign.UUID
		}
		return denmaMsgCampaign
	case denmaMsgOther:
		return "transactional"
	}
	return kind
}

// denmaTagValue keeps the characters an SES tag value may have, at most 256.
func denmaTagValue(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_':
			return r
		}
		return -1
	}, s)
	if len(s) > 256 {
		s = s[:256]
	}
	if s == "" {
		return "none"
	}
	return s
}
