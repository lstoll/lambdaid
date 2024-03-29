package main

import (
	"fmt"
	"regexp"
)

var (
	// reValidPublicRedirectUri is a fairly strict regular expression that must
	// match against the redirect URI for a 'public' client. It intentionally may
	// not match all URLs that are technically valid, but is it meant to match
	// all commonly constructed ones, without inadvertently falling victim to
	// parser bugs or parser inconsistencies (e.g.,
	// https://www.blackhat.com/docs/us-17/thursday/us-17-Tsai-A-New-Era-Of-SSRF-Exploiting-URL-Parser-In-Trending-Programming-Languages.pdf)
	reValidPublicRedirectURI = regexp.MustCompile(`\Ahttp://localhost(?::[0-9]{1,5})?(?:|/[A-Za-z0-9./_-]{0,1000})\z`)
)

type Client struct {
	ClientID      string   `json:"client_id" yaml:"client_id"`
	ClientSecrets []string `json:"client_secrets" yaml:"client_secrets"`
	RedirectURLs  []string `json:"redirect_urls" yaml:"redirect_urls"`
	Public        bool     `json:"public" yaml:"public"`
}

type clientList []Client

func (s clientList) IsValidClientID(clientID string) (ok bool, err error) {
	for _, c := range s {
		if c.ClientID == clientID {
			return true, nil
		}
	}
	return false, nil
}

func (s clientList) IsUnauthenticatedClient(clientID string) (ok bool, err error) {
	return false, nil
}

func (s clientList) ValidateClientSecret(clientID, clientSecret string) (ok bool, err error) {
	for _, c := range s {
		if c.ClientID == clientID {
			for _, cs := range c.ClientSecrets {
				if cs == clientSecret {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (s clientList) ValidateClientRedirectURI(clientID, redirectURI string) (ok bool, err error) {
	var cl Client
	var found bool
	for _, c := range s {
		if c.ClientID == clientID {
			cl = c
			found = true
		}
	}
	if !found {
		return false, fmt.Errorf("invalid client")
	}
	if cl.Public && reValidPublicRedirectURI.MatchString(redirectURI) {
		return true, nil
	}

	for _, rurl := range cl.RedirectURLs {
		if rurl == redirectURI {
			return true, nil
		}
	}

	return false, nil
}
