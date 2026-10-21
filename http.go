package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	domainRegexp = regexp.MustCompile(`(?i)^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)
	scriptRegexp = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	// Data blocks like __NEXT_DATA__ or ld+json never execute, and often hold
	// the full article text, so disable_js keeps them for inject_html to use
	jsonScriptRegexp = regexp.MustCompile(`(?i)^<script\b[^>]*\btype\s*=\s*["']?application/(ld\+)?json`)
	bodyRegexp   = regexp.MustCompile(`(?i)<body\b[^>]*>`)
)

// Mitm warning banner & menu. A menu.html next to the binary overrides it
//
//go:embed menu.html
var menuHTML []byte

const archiveHost = "web.archive.org"

func mainHandler(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if host == config.InfoHost {
		infoHandler(w, r)
		return
	}

	options := getHostOptions(host)
	if options == nil {
		w.Write([]byte("You shall not pass!"))
		return
	}
	fromArchive := enabled(options.FromArchive, false)

	newUrl := &url.URL{Scheme: "https", Host: host}
	proxy := httputil.NewSingleHostReverseProxy(newUrl)
	director := proxy.Director
	proxy.Transport = transport
	if fromArchive {
		proxy.Transport = archiveTransport{transport}
	}
	proxy.Director = func(req *http.Request) {
		// Use the original director to forward the request to the original host
		director(req)
		req.Host = host

		// Let the transport negotiate gzip and decode it for us. Browsers
		// also offer encodings like zstd that we can't patch
		req.Header.Del("Accept-Encoding")

		// pull latest snapshot from the wayback machine. id_ returns the
		// original page without the archive toolbar or rewritten links
		if fromArchive {
			log.Println("Pulling from web archive:", req.URL)
			req.URL, _ = url.Parse(fmt.Sprintf("https://%s/web/2id_/%s", archiveHost, req.URL))
			req.Host = archiveHost
			req.Header.Del("Cookie")
			return
		}

		//spoof twitter referer
		if enabled(options.SocialReferer, true) {
			req.Header.Set("Referer", "https://t.co/")
		}
		//spoof google bot ua
		if enabled(options.GooglebotUA, true) {
			req.Header.Set("User-Agent", "AdsBot-Google (+http://www.google.com/adsbot.html)")
		}
		//spoof google bot datacenter ip
		if enabled(options.GooglebotIP, true) {
			req.Header.Set("X-Forwarded-For", "66.102.0.0")
		}
		//disable cookies
		if enabled(options.DisableCookies, true) {
			req.Header.Del("Cookie")
		}
	}
	proxy.ModifyResponse = func(res *http.Response) error {

		//remove HSTS
		res.Header.Del("Strict-Transport-Security")
		if fromArchive || enabled(options.DisableCookies, true) {
			res.Header.Del("Set-Cookie")
		}

		contentType := res.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") || !canDecode(res) {
			return nil
		}
		b, err := decompress(res)
		if err != nil {
			return err
		}

		// Disable JS
		if enabled(options.DisableJS, true) {
			b = removeScripts(b)
		}

		//Inject custom html
		if options.InjectHTML != nil {
			b = injectHtml(b, *options.InjectHTML)
		}

		b = injectHtml(b, string(menuHTML))
		// A CSP would block the injected menu script
		res.Header.Del("Content-Security-Policy")

		setBody(res, b)
		return nil

	}
	proxy.ServeHTTP(w, r)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ca.pem" {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="freenews-ca.pem"`)
		w.Write([]byte(caString))
		return
	}

	if r.URL.Path == "/addhost" && r.Method == "POST" {
		r.ParseForm()
		name := strings.ToLower(strings.TrimSpace(r.Form.Get("name")))

		// Validate user input
		if !domainRegexp.MatchString(name) {
			http.Error(w, "Error: Invalid domain", http.StatusBadRequest)
			return
		}
		if getHostOptions(name) != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		hostsMu.Lock()
		defer hostsMu.Unlock()

		// Append new host to config file
		f, err := os.OpenFile("config.toml", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		_, err = f.WriteString(fmt.Sprintf("\n[[host]]\nname = \"%s\"\n", name))
		if err != nil {
			log.Println(err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}

		// Add new domain to host config
		config.Hosts = append(config.Hosts, HostOptions{
			Name: name,
		})
		log.Println("[*] Added host:", name)

		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	hosts := ""
	hostsMu.RLock()
	for _, host := range config.Hosts {
		hosts += fmt.Sprintf("%s\n", html.EscapeString(host.Name))
	}
	hostsMu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(
		"<meta name=\"viewport\" content=\"width=device-width\"><pre>Welcome to %s (DNS %s:%d)\nPlease make sure to <a href=\"/ca.pem\">install</a> the following CA:\n\n%s\n\nCurrently unlocking:\n%s</pre><br><form method=\"POST\" action=\"/addhost\"><input placeholder=\"domain.com\" required name=\"name\"><br><input type=\"submit\" value=\"Add domain\"></form>",
		html.EscapeString(config.InfoHost),
		html.EscapeString(*publicIP),
		*dnsPort,
		caString,
		hosts,
	)))
}

// archiveTransport follows the wayback machine's redirects to the closest
// snapshot itself, so the browser never leaves the original domain
type archiveTransport struct {
	rt http.RoundTripper
}

func (t archiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for i := 0; ; i++ {
		res, err := t.rt.RoundTrip(req)
		if err != nil || i == 10 {
			return res, err
		}
		loc, err := res.Location()
		if err != nil || res.StatusCode < 300 || res.StatusCode > 399 || loc.Host != archiveHost {
			return res, nil
		}
		res.Body.Close()
		req = req.Clone(req.Context())
		req.URL = loc
		req.Host = loc.Host
	}
}

func removeScripts(b []byte) []byte {
	return scriptRegexp.ReplaceAllFunc(b, func(script []byte) []byte {
		if jsonScriptRegexp.Match(script) {
			return script
		}
		return nil
	})
}

// injectHtml inserts inject right after the opening <body> tag
func injectHtml(b []byte, inject string) []byte {
	loc := bodyRegexp.FindIndex(b)
	if loc == nil {
		return b
	}
	out := make([]byte, 0, len(b)+len(inject))
	out = append(out, b[:loc[1]]...)
	out = append(out, inject...)
	return append(out, b[loc[1]:]...)
}

func serveHTTP() {
	if custom, err := os.ReadFile("./menu.html"); err == nil {
		menuHTML = custom
	}

	//create custom resolver that does not use our own dns
	dnsResolverIP := config.UpstreamDNS
	dnsResolverProto := "udp"
	dnsResolverTimeoutMs := 3000
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Duration(dnsResolverTimeoutMs) * time.Millisecond,
				}
				return d.DialContext(ctx, dnsResolverProto, dnsResolverIP)
			},
		},
	}

	transport = &http.Transport{
		DialContext:         dialer.DialContext,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 4,
	}

	go func() {
		server := &http.Server{
			Addr:              fmt.Sprintf(":%d", *httpPort),
			Handler:           http.HandlerFunc(mainHandler),
			ReadHeaderTimeout: 30 * time.Second,
		}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("[ERR] %s . Terminating.", err)
		}
	}()
	log.Printf("[HTTP] Listening on 0.0.0.0:%d/%d(tls)", *httpPort, *httpsPort)
	if err := listenAndServeTLS(); err != nil {
		log.Fatalf("[ERR] %s . Terminating.", err)
	}
}

func listenAndServeTLS() error {
	conn, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *httpsPort))
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(conn, tlsHttpServerConfig)
	server := &http.Server{
		Handler:           http.HandlerFunc(mainHandler),
		ReadHeaderTimeout: 30 * time.Second,
	}

	return server.Serve(tlsListener)
}
