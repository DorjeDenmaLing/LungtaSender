package main

// denma: a campaign's From can be set to the center's contact e-mail (denma.contact_email).

import (
	"net/mail"
	"strings"
)

// denmaFromView is what the campaign page needs to offer the center's e-mail as the From.
type denmaFromView struct {
	Email  string `json:"email"`  // the center's contact e-mail, "" in the hub
	Name   string `json:"name"`   // the From's name when the campaign's From has none
	Usable bool   `json:"usable"` // the center may send as it
}

// denmaCenterFrom is the center's contact e-mail, as a campaign's From.
func (a *App) denmaCenterFrom() denmaFromView {
	email := strings.TrimSpace(a.ko.String("denma.contact_email"))
	if email == "" || a.denmaCenterOf() == nil {
		return denmaFromView{}
	}
	name := a.cfg.SiteName
	if addr, err := mail.ParseAddress(a.cfg.FromEmail); err == nil && addr.Name != "" {
		name = addr.Name
	}
	return denmaFromView{Email: email, Name: name, Usable: a.denmaCheckSender(email) == nil}
}
