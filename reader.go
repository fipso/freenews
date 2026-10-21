package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Reader mode: paste an article URL, get the unpaywalled text on a clean page.
// Needs no CA or DNS changes on the device, unlike the reverse proxy.

const (
	maxPageBytes  = 10 << 20
	minArticleLen = 150 // words. Below this, try the wayback machine too
)

var readerClient *http.Client

var errPrivateAddress = errors.New("that address is on a private network, so it can't be read")

// Interesting keys in embedded JSON (__NEXT_DATA__, ld+json, ...)
var jsonBodyKeys = map[string]bool{
	"body": true, "articleBody": true, "content": true, "bodyHtml": true,
	"html": true, "text": true, "articleText": true, "contentHtml": true,
}

var junkRegexp = regexp.MustCompile(`(?i)paywall|subscri|advert|newsletter|related|share|social|promo|comment|signup|sign-up|recommend|byline|toolbar|breadcrumb`)

type article struct {
	URL     string
	Title   string
	Site    string
	Image   string
	Content template.HTML
	Words   int
	Source  string
}

func serveReader(port int) {
	readerClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: newReaderTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", readerHomeHandler)
	mux.HandleFunc("/read", readHandler)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	log.Printf("[READER] Listening on http://%s:%d", *publicIP, port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[ERR] Reader: %s . Terminating.", err)
	}
}

// newReaderTransport resolves via the upstream DNS (not our own, which would
// point unpaywalled hosts back at us) and refuses private addresses, so the
// reader can't be used to poke at the local network
func newReaderTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, "udp", config.UpstreamDNS)
			},
		},
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || !isPublicIP(ip) {
				return errPrivateAddress
			}
			return nil
		},
	}
	return &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.Equal(net.IPv4bcast) ||
		// Carrier grade NAT
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64))
}

func readerHomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderReader(w, http.StatusOK, readerPage{Bookmarklet: bookmarklet(r)})
}

func readHandler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	// Pasted text from share sheets often has a title in front of the link
	if i := strings.Index(raw, "http"); i > 0 {
		raw = strings.Fields(raw[i:])[0]
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	page := readerPage{Input: raw, Bookmarklet: bookmarklet(r)}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		page.Error = "That doesn't look like a link to an article."
		renderReader(w, http.StatusBadRequest, page)
		return
	}

	a, err := fetchArticle(r.Context(), u)
	if err != nil {
		log.Println("[READER]", u, err)
		page.Error = "Couldn't load the article: " + err.Error() + "."
		var wall *botWallError
		if errors.As(err, &wall) {
			page.Error = "This site blocks automated readers: " + err.Error() + "."
		}
		renderReader(w, http.StatusBadGateway, page)
		return
	}
	log.Printf("[READER] %s: %d words from %s", u, a.Words, a.Source)
	page.Article = a
	renderReader(w, http.StatusOK, page)
}

func bookmarklet(r *http.Request) template.URL {
	return template.URL(fmt.Sprintf(
		"javascript:location.href='http://%s/read?url='+encodeURIComponent(location.href)", r.Host))
}

func fetchArticle(ctx context.Context, u *url.URL) (*article, error) {
	options := getHostOptions(strings.ToLower(u.Hostname()))
	if options == nil {
		options = &HostOptions{}
	}

	if ip := net.ParseIP(u.Hostname()); ip != nil && !isPublicIP(ip) {
		return nil, errPrivateAddress
	}

	// Bot tricks first, then as a normal browser (some sites block fake
	// Googlebots), then the wayback machine. Stop once we have an article
	modes := []fetchMode{modeTricks, modeBrowser, modeArchive}
	if enabled(options.FromArchive, false) {
		modes = []fetchMode{modeArchive, modeTricks, modeBrowser}
	}

	var best *article
	var firstErr error
	var wall *botWallError
	archiveFailed := false
	for _, mode := range modes {
		a, err := fetchAndExtract(ctx, u, options, mode)
		if errors.Is(err, errPrivateAddress) {
			return nil, err
		}
		if err != nil {
			log.Printf("[READER] %s (%s): %s", u, mode, err)
			if firstErr == nil {
				firstErr = err
			}
			if mode == modeArchive {
				archiveFailed = true
			} else if wall == nil {
				errors.As(err, &wall)
			}
			continue
		}
		if best == nil || a.Words > best.Words {
			best = a
		}
		if best.Words >= minArticleLen {
			break
		}
	}
	if best == nil {
		// The bot wall is the real reason, not e.g. a missing archive copy
		if wall != nil {
			wall.NoArchive = archiveFailed
			return nil, wall
		}
		return nil, firstErr
	}
	return best, nil
}

// Bot protection services answer with a challenge page instead of the
// article. We don't try to get past them, only explain what happened
var botWalls = []struct{ vendor, marker string }{
	{"DataDome", "captcha-delivery.com"},
	{"DataDome", "datadome"},
	{"Cloudflare", "challenges.cloudflare.com"},
	{"Cloudflare", "cf_chl_opt"},
	{"Cloudflare", "cf-chl-"},
	{"Imperva", "_incapsula_resource"},
	{"HUMAN (PerimeterX)", "px-captcha"},
	{"HUMAN (PerimeterX)", "perimeterx"},
}

// detectBotWall returns the bot protection vendor if page is a challenge
// page. Only short pages count: a real article may mention these names
func detectBotWall(page []byte) string {
	if len(page) > 64<<10 {
		return ""
	}
	low := bytes.ToLower(page)
	for _, w := range botWalls {
		if bytes.Contains(low, []byte(w.marker)) {
			return w.vendor
		}
	}
	return ""
}

type botWallError struct {
	Site, Vendor string
	NoArchive    bool
}

func (e *botWallError) Error() string {
	msg := fmt.Sprintf("%s is protected by %s, a bot blocker that only lets real browsers through (it answers automated readers like this one with a CAPTCHA)", e.Site, e.Vendor)
	if e.NoArchive {
		msg += ". The Wayback Machine has no copy either, so this article can only be opened in a normal browser"
	}
	return msg
}

type fetchMode string

const (
	modeTricks  fetchMode = "bot tricks"
	modeBrowser fetchMode = "browser"
	modeArchive fetchMode = "archive"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"

func fetchAndExtract(ctx context.Context, u *url.URL, options *HostOptions, mode fetchMode) (*article, error) {
	target := u.String()
	if mode == modeArchive {
		target = fmt.Sprintf("https://%s/web/2id_/%s", archiveHost, u)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", browserUA)
	if mode == modeTricks {
		// Same tricks as the reverse proxy
		if enabled(options.SocialReferer, true) {
			req.Header.Set("Referer", "https://t.co/")
		}
		if enabled(options.GooglebotUA, true) {
			req.Header.Set("User-Agent", "AdsBot-Google (+http://www.google.com/adsbot.html)")
		}
		if enabled(options.GooglebotIP, true) {
			req.Header.Set("X-Forwarded-For", "66.102.0.0")
		}
	}

	res, err := readerClient.Do(req)
	if err != nil {
		if errors.Is(err, errPrivateAddress) {
			return nil, errPrivateAddress
		}
		return nil, errors.New("the site didn't respond or refused the connection")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxPageBytes))
	if err != nil {
		return nil, err
	}
	if vendor := detectBotWall(b); vendor != "" {
		return nil, &botWallError{Site: strings.TrimPrefix(res.Request.URL.Hostname(), "www."), Vendor: vendor}
	}
	if res.StatusCode != http.StatusOK {
		if mode == modeArchive && res.StatusCode == http.StatusNotFound {
			return nil, errors.New("the Wayback Machine has no copy")
		}
		return nil, fmt.Errorf("%s answered %s", res.Request.URL.Host, res.Status)
	}
	if ct := res.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "html") {
		return nil, fmt.Errorf("not a web page (%s)", ct)
	}

	a := extractArticle(b, u)
	a.Source = "the page"
	if mode == modeArchive {
		a.Source = "the Wayback Machine"
	}
	return a, nil
}

// extractArticle picks the longest of: article text hidden in embedded JSON,
// or the page element with the most paragraph text
func extractArticle(page []byte, base *url.URL) *article {
	a := &article{URL: base.String(), Site: strings.TrimPrefix(base.Hostname(), "www.")}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return a
	}

	var scripts []string
	var ogTitle, title, h1 string
	var best *html.Node
	scores := map[*html.Node]float64{}
	walk(doc, func(n *html.Node) bool {
		switch n.DataAtom {
		case atom.Meta:
			switch attr(n, "property") {
			case "og:title":
				ogTitle = attr(n, "content")
			case "og:image":
				a.Image = resolveURL(base, attr(n, "content"))
			case "og:site_name":
				a.Site = attr(n, "content")
			}
		case atom.Title:
			if title == "" {
				title = textOf(n)
			}
		case atom.H1:
			if h1 == "" {
				h1 = textOf(n)
			}
		case atom.Script:
			if strings.Contains(strings.ToLower(attr(n, "type")), "json") && n.FirstChild != nil {
				scripts = append(scripts, n.FirstChild.Data)
			}
			return false
		case atom.P:
			// Credit paragraph text to its container, and less to the one above
			words := float64(countWords(textOf(n)))
			if words < 5 {
				break
			}
			if p := n.Parent; p != nil {
				scores[p] += words
				if p.Parent != nil {
					scores[p.Parent] += words / 2
				}
			}
		}
		return true
	})
	for n, s := range scores {
		if best == nil || s > scores[best] {
			best = n
		}
	}

	a.Title = firstNonEmpty(ogTitle, h1, title, a.Site)

	if best != nil {
		var buf bytes.Buffer
		sanitizeChildren(&buf, best, base)
		a.Content = template.HTML(buf.String())
		a.Words = countWords(htmlText(buf.String()))
	}

	for _, s := range scripts {
		body := longestJSONBody(s)
		if body == "" {
			continue
		}
		var buf bytes.Buffer
		if strings.Contains(body, "<") {
			if frag, err := html.Parse(strings.NewReader(body)); err == nil {
				sanitizeChildren(&buf, frag, base)
			}
		} else {
			for _, para := range strings.Split(body, "\n") {
				if para = strings.TrimSpace(para); para != "" {
					fmt.Fprintf(&buf, "<p>%s</p>\n", html.EscapeString(para))
				}
			}
		}
		if words := countWords(htmlText(buf.String())); words > a.Words {
			a.Content = template.HTML(buf.String())
			a.Words = words
		}
	}
	return a
}

// longestJSONBody finds the longest article-like string in a JSON document
func longestJSONBody(s string) string {
	var v interface{}
	if json.Unmarshal([]byte(s), &v) != nil {
		return ""
	}
	best, bestWords := "", 0
	var visit func(key string, v interface{})
	visit = func(key string, v interface{}) {
		switch v := v.(type) {
		case map[string]interface{}:
			for k, c := range v {
				visit(k, c)
			}
		case []interface{}:
			for _, c := range v {
				visit(key, c)
			}
		case string:
			if jsonBodyKeys[key] {
				if w := countWords(htmlText(v)); w > bestWords {
					best, bestWords = v, w
				}
			}
		}
	}
	visit("", v)
	return best
}

// Tags kept as is. Everything else not in dropTags is unwrapped
var keepTags = map[atom.Atom]bool{
	atom.P: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.Blockquote: true,
	atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Strong: true, atom.B: true,
	atom.Em: true, atom.I: true, atom.Br: true, atom.Figure: true, atom.Figcaption: true,
	atom.A: true, atom.Img: true, atom.Pre: true, atom.Code: true, atom.Hr: true,
	atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tr: true, atom.Td: true, atom.Th: true,
}

var dropTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Iframe: true,
	atom.Form: true, atom.Button: true, atom.Input: true, atom.Select: true,
	atom.Textarea: true, atom.Nav: true, atom.Aside: true, atom.Footer: true,
	atom.Header: true, atom.Svg: true, atom.Object: true, atom.Embed: true,
	atom.Template: true, atom.Dialog: true, atom.Head: true, atom.Title: true,
}

func sanitizeChildren(buf *bytes.Buffer, n *html.Node, base *url.URL) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sanitize(buf, c, base)
	}
}

// sanitize writes n as safe HTML: an allowlist of tags, no attributes but
// http(s) links and image sources, so nothing from the page can run
func sanitize(buf *bytes.Buffer, n *html.Node, base *url.URL) {
	switch n.Type {
	case html.TextNode:
		buf.WriteString(html.EscapeString(n.Data))
		return
	case html.ElementNode:
	default:
		sanitizeChildren(buf, n, base)
		return
	}

	if dropTags[n.DataAtom] || junkRegexp.MatchString(attr(n, "class")+" "+attr(n, "id")) {
		return
	}
	tag := n.DataAtom
	if tag == atom.H1 {
		tag = atom.H2
	}
	if !keepTags[tag] {
		sanitizeChildren(buf, n, base)
		return
	}

	switch tag {
	case atom.Img:
		src := attr(n, "src")
		if src == "" || strings.HasPrefix(src, "data:") {
			src = firstNonEmpty(attr(n, "data-src"), attr(n, "data-original"), firstSrcset(attr(n, "srcset")))
		}
		if src = resolveURL(base, src); src != "" {
			fmt.Fprintf(buf, `<img src="%s" alt="%s" loading="lazy">`, html.EscapeString(src), html.EscapeString(attr(n, "alt")))
		}
		return
	case atom.Br, atom.Hr:
		fmt.Fprintf(buf, "<%s>", tag)
		return
	case atom.A:
		if href := resolveURL(base, attr(n, "href")); href != "" {
			fmt.Fprintf(buf, `<a href="%s" rel="noopener noreferrer" target="_blank">`, html.EscapeString(href))
			sanitizeChildren(buf, n, base)
			buf.WriteString("</a>")
			return
		}
		sanitizeChildren(buf, n, base)
		return
	}

	fmt.Fprintf(buf, "<%s>", tag)
	sanitizeChildren(buf, n, base)
	fmt.Fprintf(buf, "</%s>", tag)
}

func resolveURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := base.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

func firstSrcset(srcset string) string {
	if f := strings.Fields(srcset); len(f) > 0 {
		return strings.TrimSuffix(f[0], ",")
	}
	return ""
}

// walk visits n and its descendants. visit returns false to skip children
func walk(n *html.Node, visit func(*html.Node) bool) {
	if n.Type == html.ElementNode && !visit(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, visit)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

var tagStripRegexp = regexp.MustCompile(`<[^>]*>`)

func htmlText(s string) string {
	return tagStripRegexp.ReplaceAllString(s, " ")
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

type readerPage struct {
	Input       string
	Error       string
	Article     *article
	Bookmarklet template.URL
}

func renderReader(w http.ResponseWriter, status int, page readerPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Article images load from anywhere, nothing else does
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src http: https:; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	if err := readerTemplate.Execute(w, page); err != nil {
		log.Println("[READER]", err)
	}
}

var readerTemplate = template.Must(template.New("reader").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .Article}}{{.Article.Title}} · {{end}}FreeNews Reader</title>
<style>
:root{--bg:#fbfaf7;--fg:#1d1d1f;--muted:#6e6e73;--line:#e3e1dc;--accent:#b4232c;--card:#fff}
@media (prefers-color-scheme:dark){:root{--bg:#161616;--fg:#e8e6e1;--muted:#9a9894;--line:#2e2e2e;--accent:#ff6b6b;--card:#1f1f1f}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
.wrap{max-width:720px;margin:0 auto;padding:0 16px}
header{border-bottom:1px solid var(--line);padding:14px 0;margin-bottom:28px}
header .wrap{display:flex;flex-wrap:wrap;gap:10px;align-items:center}
.brand{font-weight:700;color:var(--fg);text-decoration:none;margin-right:auto}
.brand span{color:var(--accent)}
form{display:flex;gap:8px;width:100%}
input[type=url],input[type=text]{flex:1;min-width:0;font:inherit;padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--fg)}
button{font:inherit;font-weight:600;padding:10px 16px;border:0;border-radius:8px;background:var(--accent);color:#fff;cursor:pointer}
.home{padding:12vh 0 40px;text-align:center}
.home h1{font-size:2rem;margin:0 0 8px}
.home p{color:var(--muted);margin:0 0 24px}
.tips{margin-top:40px;text-align:left;color:var(--muted);font-size:.92rem;border-top:1px solid var(--line);padding-top:20px}
.tips a{color:var(--accent)}
.error{background:var(--card);border:1px solid var(--accent);border-radius:8px;padding:12px 14px;margin:0 0 20px}
article h1{font:700 2rem/1.2 Georgia,"Times New Roman",serif;margin:0 0 10px}
.meta{color:var(--muted);font-size:.9rem;margin-bottom:24px}
.meta a{color:var(--muted)}
.content{font:1.2rem/1.7 Georgia,"Times New Roman",serif;overflow-wrap:break-word}
.content img{max-width:100%;height:auto;border-radius:6px}
.content figure{margin:24px 0}
.content figcaption{font:.85rem/1.4 system-ui,sans-serif;color:var(--muted);margin-top:6px}
.content a{color:var(--accent)}
.content blockquote{border-left:3px solid var(--line);margin:0;padding-left:16px;color:var(--muted)}
.content table{display:block;overflow-x:auto;border-collapse:collapse;font-size:.95rem}
.content td,.content th{border:1px solid var(--line);padding:4px 8px}
.lead{width:100%;border-radius:8px;margin-bottom:20px}
.short{color:var(--muted);font-style:italic;border-top:1px solid var(--line);padding-top:16px;margin-top:32px}
footer{color:var(--muted);font-size:.85rem;padding:40px 0}
</style>
</head>
<body>
<header><div class="wrap">
<a class="brand" href="/">Free<span>News</span> Reader</a>
{{if or .Article .Error}}<form action="/read" method="get"><input type="text" name="url" placeholder="Paste article link" value="{{.Input}}" required><button>Read</button></form>{{end}}
</div></header>
<main class="wrap">
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
{{with .Article}}
<article>
<h1>{{.Title}}</h1>
<div class="meta">{{.Site}} · {{.Words}} words from {{.Source}} · <a href="{{.URL}}" rel="noopener noreferrer" target="_blank">Original</a></div>
{{if .Image}}<img class="lead" src="{{.Image}}" alt="">{{end}}
<div class="content">{{.Content}}</div>
{{if lt .Words 150}}<p class="short">Only a short excerpt could be found. This site may need extra settings in config.toml.</p>{{end}}
</article>
{{else}}{{if not .Error}}
<section class="home">
<h1>Paste a link, read the article</h1>
<p>Unpaywalled, without ads, scripts or trackers.</p>
<form action="/read" method="get"><input type="text" name="url" placeholder="https://asia.nikkei.com/..." autofocus required><button>Read</button></form>
<div class="tips">
<p><b>Faster:</b> drag this to your bookmarks bar, then click it on any article: <a href="{{.Bookmarklet}}">Read with FreeNews</a></p>
<p>On your phone, add this page to your home screen.</p>
</div>
</section>
{{end}}{{end}}
</main>
<footer class="wrap">Text is fetched by your own FreeNews server.</footer>
</body>
</html>
`))
