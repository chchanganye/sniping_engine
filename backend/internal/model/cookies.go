package model

import (
	"net/http"
	"time"
)

type CookieJarEntry struct {
	URL     string   `json:"url"`
	Cookies []Cookie `json:"cookies"`
}

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

func CookiesFromHTTP(in []*http.Cookie) []Cookie {
	out := make([]Cookie, 0, len(in))
	for _, c := range in {
		var expires int64
		if !c.Expires.IsZero() {
			expires = c.Expires.UnixMilli()
		}
		out = append(out, Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Expires:  expires,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
			SameSite: sameSiteToString(c.SameSite),
		})
	}
	return out
}

func CookiesToHTTP(in []Cookie) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(in))
	for _, c := range in {
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
			SameSite: sameSiteFromString(c.SameSite),
		}
		if c.Expires > 0 {
			hc.Expires = time.UnixMilli(c.Expires)
		}
		out = append(out, hc)
	}
	return out
}

func sameSiteToString(s http.SameSite) string {
	switch s {
	case http.SameSiteDefaultMode:
		return "default"
	case http.SameSiteLaxMode:
		return "lax"
	case http.SameSiteStrictMode:
		return "strict"
	case http.SameSiteNoneMode:
		return "none"
	default:
		return "default"
	}
}

func sameSiteFromString(s string) http.SameSite {
	switch s {
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}

