package main

import (
	"context"
	"crypto/tls"
	"fmt"
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

var domainRegexp = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9-]{1,61}[a-zA-Z0-9]\.)+[a-zA-Z]{2,}$`)

func mainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Host == config.InfoHost {

		if r.URL.Path == "/ca.pem" {
			w.Header().Set("Content-Type", "application/x-pem-file")
			w.Write([]byte(caString))
			return
		}

		if r.URL.Path == "/addhost" && r.Method == "POST" {
			r.ParseForm()
			name := r.Form.Get("name")

			// Validate user input
			if !domainRegexp.MatchString(name) {
				http.Error(w, "Error: Invalid domain", http.StatusBadRequest)
				return
			}

			// Add new domain to host config
			config.Hosts = append(config.Hosts, HostOptions{
				Name: name,
			})

			// Append new host to config file
			f, err := os.OpenFile("config.toml", os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				log.Println(err)
				w.Write([]byte("Error"))
				return
			}
			defer f.Close()

			_, err = f.WriteString(fmt.Sprintf("\n[[host]]\nname = \"%s\"\n", name))
			if err != nil {
				log.Println(err)
				w.Write([]byte("Error"))
				return
			}

			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		hosts := ""
		for _, host := range config.Hosts {
			hosts += fmt.Sprintf("%s\n", host.Name)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(
			"<pre>Welcome to %s (DNS %s:%d)\nPlease make sure to <a href=\"/ca.pem\">install<a/> the following CA:\n\n%s\n\nCurrently unlocking:\n%s</pre><br><form method=\"POST\" action=\"/addhost\"><input placeholder=\"domain.com\" required name=\"name\"><br><input type=\"submit\" value=\"Add domain\"></form>",
			config.InfoHost,
			*publicIP,
			*dnsPort,
			caString,
			hosts,
		)))
		return
	}

	options := getHostOptions(r.Host)
	if options == nil {
		w.Write([]byte("You shall not pass!"))
		return
	}

	newUrl, err := url.Parse(fmt.Sprintf("https://%s", r.Host))
	if err != nil {
		w.Write([]byte("Could not parse URL"))
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(newUrl)
	director := proxy.Director
	proxy.Transport = transport
	proxy.Director = func(req *http.Request) {
		// Use the original director to forward the request to the original host
		director(req)

		// pull page from google cache
		if options.FromGoogleCache != nil && *options.FromGoogleCache {
			log.Println("Pulling from google cache")
			req.URL, _ = url.Parse(
				"https://webcache.googleusercontent.com/search?q=cache:" + req.URL.String(),
			)
			req.Host = "webcache.googleusercontent.com"
			return
		}

		//spoof twitter referer
		if options.SocialReferer == nil || *options.SocialReferer {
			req.Header.Set("Referer", "https://t.co/")
		}
		//spoof google bot ua
		if options.GooglebotUA == nil || *options.GooglebotUA {
			req.Header.Set("User-Agent", "AdsBot-Google (+http://www.google.com/adsbot.html)")
		}
		//spoof google bot datacenter ip
		if options.GooglebotIP == nil || *options.GooglebotIP {
			req.Header.Set("X-Forwarded-For", "66.102.0.0")
		}
		//disable cookies
		if options.DisableCookies == nil || *options.DisableCookies {
			req.Header.Set("Cookie", "")
			req.Header.Set("Set-Cookie", "")
		}
	}
	proxy.ModifyResponse = func(res *http.Response) error {

		//remove HTST
		res.Header.Set("Strict-Transport-Security", "")

		contentType := res.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			return nil
		}
		b, err := decompress(res)
		if err != nil {
			return err
		}

		// Remove Google Cache Banner & Stylesheet
		if options.FromGoogleCache != nil && *options.FromGoogleCache {
			// TODO: Simplify this regex
			re := regexp.MustCompile(
				`<!doctype html>(.|\n)*?<div id="...........__google-cache-hdr">(.|\n)*?<!doctype html>`,
			)
			b = re.ReplaceAll(b, []byte(""))
		}

		// Disable JS
		if options.DisableJS == nil || *options.DisableJS {
			re := regexp.MustCompile(`<script(.|\n)*?<\/script>`)
			b = re.ReplaceAll(b, []byte(""))
		}

		//Inject custom html
		if options.InjectHTML != nil {
			b = injectHtml(b, *options.InjectHTML)
		}

		//Add mitm warning banner & menu
		menu, err := os.ReadFile("./menu.html")
		if err != nil {
			return err
		}

		b = injectHtml(b, string(menu))

		compress(res, b)
		return nil

	}
	proxy.ServeHTTP(w, r)
}

func injectHtml(b []byte, inject string) []byte {
	re := regexp.MustCompile(`<body>|<body[^>]+>`)
	locs := re.FindAllIndex(b, -1)
	for _, loc := range locs {
		b = append(b[:loc[1]], append([]byte(inject), b[loc[1]:]...)...)
	}
	//log.Println(string(b[loc[0]:loc[1]+100]))
	return b
}

func serveHTTP() {
	//create custom resolver that does not use our own dns
	dnsResolverIP := config.UpstreamDNS
	dnsResolverProto := "udp"
	dnsResolverTimeoutMs := 3000
	dialer := &net.Dialer{
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
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	go func() {
		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", *httpPort),
			Handler: http.HandlerFunc(mainHandler),
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
	server := &http.Server{Handler: http.HandlerFunc(mainHandler)}

	return server.Serve(tlsListener)
}
