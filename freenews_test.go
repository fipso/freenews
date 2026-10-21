package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func setupTestConfig(t *testing.T) {
	ip, aaaa := "203.0.113.7", ""
	publicIP, mitmAAAA = &ip, &aaaa
	config = Config{
		InfoHost: "free.news",
		Hosts:    []HostOptions{{Name: "example.com"}},
	}
	blockList = parseBlockList("ads.example.org\n")
}

func TestParseBlockList(t *testing.T) {
	list := parseBlockList("# comment\r\nads.com\r\n\r\n0.0.0.0 Tracker.NET # inline\n127.0.0.1 localhost\n||pixel.io^\n! adblock comment\nfqdn.org.\n")
	want := []string{"ads.com", "tracker.net", "pixel.io", "fqdn.org"}
	if len(list) != len(want) {
		t.Fatalf("got %v, want %v", list, want)
	}
	for _, d := range want {
		if _, ok := list[d]; !ok {
			t.Errorf("missing %q in %v", d, list)
		}
	}
}

func TestDomainRegexp(t *testing.T) {
	for _, d := range []string{"ft.com", "www.nytimes.com", "a.co", "t3n.de", "sub.example.co.uk"} {
		if !domainRegexp.MatchString(d) {
			t.Errorf("%q should be valid", d)
		}
	}
	for _, d := range []string{"", "-bad.com", "evil\"]\nname=\"x.com", "no_tld", "a..com"} {
		if domainRegexp.MatchString(d) {
			t.Errorf("%q should be invalid", d)
		}
	}
}

func TestInjectHtml(t *testing.T) {
	got := string(injectHtml([]byte(`<html><BODY class="x"><p>hi</p><body></html>`), "<!--m-->"))
	want := `<html><BODY class="x"><!--m--><p>hi</p><body></html>`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := string(injectHtml([]byte("<p>no body</p>"), "x")); got != "<p>no body</p>" {
		t.Errorf("got %s", got)
	}
}

func TestScriptRegexp(t *testing.T) {
	data := `<script id="__NEXT_DATA__" type="application/json">{"a":1}</script><script type='application/ld+json'>{}</script>`
	in := "<p>a</p><SCRIPT src=x>\r\n</script ><noscript>keep</noscript><script>\nvar x\n</script>" + data + `<script type="module">x()</script>`
	got := string(removeScripts([]byte(in)))
	if got != "<p>a</p><noscript>keep</noscript>"+data {
		t.Errorf("got %q", got)
	}
}

func query(name string, qtype uint16) (*dns.Msg, bool) {
	req := new(dns.Msg)
	req.SetQuestion(name, qtype)
	m := new(dns.Msg)
	m.SetReply(req)
	return m, answerQuery(m)
}

func TestAnswerQuery(t *testing.T) {
	setupTestConfig(t)

	m, handled := query("WWW.Example.com.", dns.TypeA)
	if !handled || len(m.Answer) != 1 || m.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Errorf("unpaywalled host A: handled=%v answer=%v", handled, m.Answer)
	}

	// No -mitmAAAA: must not be forwarded upstream, or clients use the real IPv6
	m, handled = query("example.com.", dns.TypeAAAA)
	if !handled || len(m.Answer) != 0 {
		t.Errorf("unpaywalled host AAAA: handled=%v answer=%v", handled, m.Answer)
	}

	m, handled = query("ads.example.org.", dns.TypeA)
	if !handled || m.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("blocked host: handled=%v answer=%v", handled, m.Answer)
	}

	if _, handled = query("google.com.", dns.TypeA); handled {
		t.Error("unrelated host should be forwarded")
	}
}

func TestLeafCerts(t *testing.T) {
	setupTestConfig(t)
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	setupCerts()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(caString))

	// Hosts added at runtime get certs without a restart
	config.Hosts = append(config.Hosts, HostOptions{Name: "ft.com"})
	seen := map[string]bool{}
	for _, name := range []string{"www.ft.com", "example.com", "free.news"} {
		cert, err := tlsHttpServerConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cert.Leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: pool}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		serial := cert.Leaf.SerialNumber.String()
		if seen[serial] {
			t.Errorf("duplicate serial %s", serial)
		}
		seen[serial] = true
	}

	if _, err := tlsHttpServerConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "bank.com"}); err == nil ||
		!strings.Contains(err.Error(), "not on unpaywall list") {
		t.Errorf("should refuse certs for unlisted hosts, got %v", err)
	}

	// Restart: CA is loaded back from disk
	ca, caString = nil, ""
	setupCerts()
	if !pool.Equal(func() *x509.CertPool { p := x509.NewCertPool(); p.AddCert(ca); return p }()) {
		t.Error("reloaded CA differs")
	}
}
