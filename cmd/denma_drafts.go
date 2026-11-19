package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// What a campaign's fields are checked for.
const (
	denmaCheckSend  = iota // a campaign that's scheduled, paused or made by the sending API
	denmaCheckDraft        // a draft: no subject or recipients yet
	denmaCheckTest         // a test e-mail: no recipients
)

const denmaUntitled = "Untitled campaign"

var errDenmaNoSubject = errors.New("Add a subject (Campaign tab) before sending.")

// denmaCheckFor is the check for a campaign saved with this status.
func denmaCheckFor(status string) int {
	if status == models.CampaignStatusDraft {
		return denmaCheckDraft
	}
	return denmaCheckSend
}

// denmaFillDraft names an unnamed draft or test.
func (c *campReq) denmaFillDraft() {
	if c.denmaCheck != denmaCheckSend && strings.TrimSpace(c.Name) == "" {
		c.Name = denmaUntitled
	}
}

// denmaSubjectMin is a subject's shortest length: a draft may have none.
func (c campReq) denmaSubjectMin() int {
	if c.denmaCheck == denmaCheckDraft {
		return 0
	}
	return 1
}

// denmaNeedsRecipients says whether lists or subscriber tags are required.
func (c campReq) denmaNeedsRecipients() bool {
	return c.denmaCheck == denmaCheckSend
}

// denmaKeepLists gives a save that leaves out its lists campaign cm's.
func denmaKeepLists(o *campReq, cm models.Campaign) {
	if o.ListIDs != nil {
		return
	}
	o.ListIDs = []int{}
	for _, l := range cm.ParsedLists() {
		if l.ID != 0 {
			o.ListIDs = append(o.ListIDs, l.ID)
		}
	}
}

// denmaCheckSubject refuses to start campaign id without a subject.
func (a *App) denmaCheckSubject(id int) error {
	var subject string
	if err := a.db.Get(&subject, `SELECT subject FROM campaigns WHERE id = $1`, id); err != nil {
		a.log.Printf("denma: error reading campaign %d's subject: %v", id, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Error reading the campaign.")
	}
	if strings.TrimSpace(subject) == "" {
		return denmaBadRequest(errDenmaNoSubject)
	}
	return nil
}
