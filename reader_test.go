package main

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func words(n int) string {
	return strings.TrimSpace(strings.Repeat("lorem ", n))
}

func TestExtractArticleFromNextData(t *testing.T) {
	base, _ := url.Parse("https://news.example.com/a/story")
	page := `<html><head><meta property="og:title" content="Big &amp; Story">
<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"data":{
"preview":"short teaser text here",
"body":"<html><body><div><p>` + words(300) + `</p><p>Second <a href=\"/x\">link</a> <img src=\"x.jpg\" onerror=\"alert(1)\"></p>\u003cscript>alert(1)\u003c/script></div></body></html>"}}}}</script>
</head><body><article><p>` + words(40) + `</p></article></body></html>`

	a := extractArticle([]byte(page), base)
	if a.Title != "Big & Story" {
		t.Errorf("title = %q", a.Title)
	}
	if a.Words < 300 {
		t.Errorf("words = %d, want the full embedded body", a.Words)
	}
	c := string(a.Content)
	for _, bad := range []string{"<script", "onerror", "alert"} {
		if strings.Contains(c, bad) {
			t.Errorf("content contains %q: %s", bad, c)
		}
	}
	for _, want := range []string{`href="https://news.example.com/x"`, `src="https://news.example.com/a/x.jpg"`} {
		if !strings.Contains(c, want) {
			t.Errorf("content missing %s: %s", want, c)
		}
	}
}

func TestExtractArticleFromDOM(t *testing.T) {
	base, _ := url.Parse("https://example.org/")
	page := `<html><head><title>Fallback title</title></head><body>
<nav><p>` + words(30) + `</p></nav>
<div class="story"><div><p>` + words(100) + `</p></div><div><p>` + words(100) + `</p></div>
<div class="newsletter-signup"><p>Subscribe to our newsletter today please now</p></div></div>
<div class="sidebar"><p>` + words(20) + `</p></div></body></html>`

	a := extractArticle([]byte(page), base)
	if a.Title != "Fallback title" {
		t.Errorf("title = %q", a.Title)
	}
	if a.Words != 200 {
		t.Errorf("words = %d, want 200: %s", a.Words, a.Content)
	}
	if strings.Contains(string(a.Content), "Subscribe") {
		t.Error("newsletter box should be dropped")
	}
}

func TestPlainTextJSONBody(t *testing.T) {
	base, _ := url.Parse("https://example.org/")
	page := `<script type="application/ld+json">{"@type":"NewsArticle","articleBody":"` + words(200) + `\nTom & Jerry ` + words(10) + `"}</script>`
	a := extractArticle([]byte(page), base)
	if a.Words != 213 || strings.Count(string(a.Content), "<p>") != 2 || !strings.Contains(string(a.Content), "Tom &amp; Jerry") {
		t.Errorf("words=%d content=%s", a.Words, a.Content)
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "192.168.1.84", "10.0.0.1", "172.16.5.4", "169.254.169.254", "100.64.0.1", "::1", "fe80::1", "fd00::1", "0.0.0.0"} {
		if isPublicIP(net.ParseIP(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "167.82.51.52", "2606:4700::1111"} {
		if !isPublicIP(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
}

func TestDetectBotWall(t *testing.T) {
	cases := map[string]string{
		`<html><body><p id="cmsg">Please enable JS and disable any ad blocker</p><script data-cfasync="false">var dd={'host':'geo.captcha-delivery.com'}</script><script src="https://ct.captcha-delivery.com/c.js"></script></body></html>`: "DataDome",
		`<!DOCTYPE html><title>Just a moment...</title><script>window._cf_chl_opt={cvId:'3'}</script><script src="https://challenges.cloudflare.com/turnstile/v0/api.js"></script>`: "Cloudflare",
		`<html><head><META NAME="robots" CONTENT="noindex,nofollow"><script src="/_Incapsula_Resource?SWJIYLWA=5074a7"></script></head></html>`: "Imperva",
		`<div id="px-captcha"></div>`: "HUMAN (PerimeterX)",
		`<html><body><article><p>An ordinary article about Cloudflare and DataDome.</p></article></body></html>` + strings.Repeat(" ", 70<<10): "",
		`<html><body><p>Just a normal short page</p></body></html>`: "",
	}
	for page, want := range cases {
		if got := detectBotWall([]byte(page)); got != want {
			t.Errorf("detectBotWall(%.60q) = %q, want %q", page, got, want)
		}
	}

	msg := (&botWallError{Site: "kn-online.de", Vendor: "DataDome", NoArchive: true}).Error()
	for _, want := range []string{"kn-online.de", "DataDome", "CAPTCHA", "Wayback Machine has no copy"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}
