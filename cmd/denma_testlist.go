package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// denmaTestListMax is the most subscribed people a list can have to be sent a test.
const denmaTestListMax = 25

// denmaTestListSQL is list $1's subscribed members' addresses (as a campaign picks them), at most $2.
const denmaTestListSQL = `SELECT s.email FROM subscriber_lists sl
	JOIN subscribers s ON s.id = sl.subscriber_id
	JOIN lists l ON l.id = sl.list_id
	WHERE sl.list_id = $1 AND s.status != 'blocklisted'
	AND ((l.optin = 'double' AND sl.status = 'confirmed') OR (l.optin != 'double' AND sl.status != 'unsubscribed'))
	ORDER BY s.id LIMIT $2`

// denmaTestListNameSQL is list $1's name, if it's active and not a sending API campaign's own.
const denmaTestListNameSQL = `SELECT name FROM lists l WHERE id = $1 AND status = 'active'
	AND NOT EXISTS (SELECT 1 FROM denma_api_campaigns x WHERE x.list_id = l.id)`

// denmaAddTestList adds to a test's addresses those of the list it names, one the user manages.
func (a *App) denmaAddTestList(c echo.Context, req *campReq) error {
	id := req.DenmaTestList
	if id == 0 {
		return nil
	}
	if _, ok := denmaMCPOf(c); ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Through an MCP key, test e-mails go only to your own address.")
	}
	u := auth.GetUser(c)
	if ids := u.FilterListsByPerm(auth.PermTypeManage, []int{id}); len(ids) != 1 {
		return echo.NewHTTPError(http.StatusForbidden, "Tests go only to lists you manage.")
	}

	var name string
	if err := a.db.Get(&name, denmaTestListNameSQL, id); errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusBadRequest, "That list can't be sent tests.")
	} else if err != nil {
		return err
	}

	var emails []string
	if err := a.db.Select(&emails, denmaTestListSQL, id, denmaTestListMax+1); err != nil {
		return err
	}
	if len(emails) > denmaTestListMax {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Tests go to lists of at most %d subscribed people, and %s has more.", denmaTestListMax, name))
	}
	if len(emails) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Nobody on %s is subscribed.", name))
	}

	seen := map[string]bool{}
	for _, e := range req.SubscriberEmails {
		seen[strings.ToLower(strings.TrimSpace(e))] = true
	}
	for _, e := range emails {
		if k := strings.ToLower(e); !seen[k] {
			seen[k] = true
			req.SubscriberEmails = append(req.SubscriberEmails, e)
		}
	}
	return nil
}
