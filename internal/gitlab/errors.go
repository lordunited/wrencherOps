// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var htmlTag = regexp.MustCompile(`<[^>]+>`)

// explain turns a failed GitLab call into what went wrong, for a person:
// a page from a proxy in front of GitLab (HTML, not GitLab's JSON), a
// refused token, or no connection at all.
func explain(err error) string {
	var er *gitlab.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		host := ""
		if er.Response.Request != nil {
			host = er.Response.Request.URL.Host
		}
		status := er.Response.StatusCode
		body := strings.TrimSpace(string(er.Body))
		if strings.HasPrefix(strings.ToLower(body), "<") {
			// Not GitLab answering: a reverse proxy, firewall or CDN in front
			// of it. Its own words, without the markup.
			text := strings.Join(strings.Fields(htmlTag.ReplaceAllString(body, " ")), " ")
			if len(text) > 160 {
				text = text[:160] + "…"
			}
			hint := ""
			if status == http.StatusForbidden {
				hint = " This network's address is most likely not allowed through it: connect the VPN the panel normally uses, or ask for the panel server's IP to be allowed."
			}
			return fmt.Sprintf("GitLab (%s) wasn't reached: a proxy in front of it answered %d %q.%s", host, status, text, hint)
		}
		switch status {
		case http.StatusUnauthorized:
			return fmt.Sprintf("GitLab (%s) refused the panel's token (401): it's wrong, expired or revoked. Set a new GITLAB_TOKEN.", host)
		case http.StatusForbidden:
			return fmt.Sprintf("GitLab (%s) answered 403: the token is valid but can't see this (listing runners needs an admin token or the runners' scope). %s", host, er.Message)
		}
		return fmt.Sprintf("GitLab (%s) answered %d: %s", host, status, er.Message)
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.As(err, &dnsErr):
		return fmt.Sprintf("GitLab's address %q doesn't resolve: check GITLAB_BASE_URL and this network's DNS.", dnsErr.Name)
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &opErr):
		return "GitLab can't be reached from the panel (" + err.Error() + "): check the network or VPN."
	}
	return err.Error()
}
