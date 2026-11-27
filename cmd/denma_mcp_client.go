package main

// denma: the hosted MCP server forwards a client's address with their key, so bad keys count per client.

import (
	"net/netip"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	denmaMCPClientHeader    = "X-Listmonk-Client"    // the client's address
	denmaMCPConnectorHeader = "X-Listmonk-Connector" // the connector secret, which vouches for it
)

// denmaMCPClientBucket is what a client's bad keys count under: its address, if it's valid.
func denmaMCPClientBucket(addr string) string {
	a, err := netip.ParseAddr(strings.TrimSpace(addr))
	if err != nil {
		return ""
	}
	return "client:" + denmaIPKey(a.String())
}

// mcpBadBucket is what a request's bad keys count under: the connector secret's, or the client's address when the
// hosted server sends it with the connector secret, else the request's address.
func (d *denmaCenters) mcpBadBucket(c echo.Context, ip, secret string, secretOK bool) string {
	if secretOK {
		return "secret:" + secret
	}
	h := c.Request().Header
	if _, isKey := denmaBearer(h.Get(echo.HeaderAuthorization), denmaMCPKeyPrefix); isKey {
		if bucket := denmaMCPClientBucket(h.Get(denmaMCPClientHeader)); bucket != "" {
			if given := strings.TrimSpace(h.Get(denmaMCPConnectorHeader)); strings.HasPrefix(given, denmaMCPSecretPrefix) {
				var from pq.StringArray
				err := d.base.db.Get(&from, `SELECT allowed_from FROM denma.mcp_connector WHERE secret_hash = $1`, denmaAPIHash(given))
				if err == nil && denmaFromAllowed(ip, from) {
					return bucket
				}
			}
		}
	}
	return ip
}
