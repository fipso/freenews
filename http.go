package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dsnet/compress/brotli"
)

func mainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Host == "free.news" {

		if r.URL.Path == "/ca.pem" {
			w.Header().Set("Content-Type", "application/x-pem-file")
			w.Write([]byte(caString))
			return
		}

		hosts := ""
		for _, host := range config.Hosts {
			hosts += fmt.Sprintf("%s\n", host.Name)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(
			"<pre>Welcome to %s (DNS %s:%d)\nPlease make sure to <a href=\"/ca.pem\">install<a/> the following CA:\n\n%s\n\nCurrently unlocking:\n%s</pre>",
			config.InfoHost, *publicIP, *dnsPort, caString, hosts)))
		return
	}

	url, err := url.Parse("https://" + r.Host)
	if err != nil {
		log.Fatalf("[ERR] %s . Terminating.", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = transport
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		options := getHostOptions(req.Host)
		if options == nil {
			director(req)
			return
		}
		//spoof twitter referer
		if options.SocialReferer == nil || *options.SocialReferer {
			req.Header.Set("Referer", "https://t.co/")
		}
		//spoof google bot ua
		if options.SocialReferer == nil || *options.GooglebotUA {
			req.Header.Set("User-Agent", "AdsBot-Google (+http://www.google.com/adsbot.html)")
		}
		//spoof google bot datacenter ip
		if options.SocialReferer == nil || *options.GooglebotIP {
			req.Header.Set("X-Forwarded-For", "66.102.0.0")
		}
		//disable cookies
		if options.SocialReferer == nil || *options.DisableCookies {
			req.Header.Set("Cookie", "")
			req.Header.Set("Set-Cookie", "")
		}
		director(req)
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		options := getHostOptions(res.Request.Host)
		if options == nil {
			return nil
		}

		contentType := res.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			return nil
		}
		b, err := decompress(res)
		if err != nil {
			return err
		}

		//Inject custom html
		if options.InjectHTML != nil {
			b = bytes.Replace(b, []byte("<body>"), []byte(*options.InjectHTML), -1)
		}

		//Add mitm warning banner
		b = bytes.Replace(b, []byte("<body>"), []byte("<body><span style=\"background: black; font-family: Arial; font-weight: bold; width: 100% !important; display: block; text-align: center; color: white;\"> ⚠️ Content unpaywalled and relayed 🌍</span>"), -1)

		compress(res, b)
		return nil

	}
	proxy.ServeHTTP(w, r)
}

func serveHTTP() {
	//create custom resolver that does not use our own dns
	dnsResolverIP := "1.1.1.1:53"
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
		server := &http.Server{Addr: ":80", Handler: http.HandlerFunc(mainHandler)}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("[ERR] %s . Terminating.", err)
		}
	}()
	log.Println("[HTTP] Listening on 0.0.0.0:80/443(tls)")
	if err := listenAndServeTLS(); err != nil {
		log.Fatalf("[ERR] %s . Terminating.", err)
	}
}

func listenAndServeTLS() error {
	conn, err := net.Listen("tcp", "0.0.0.0:443")
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(conn, tlsHttpServerConfig)
	server := &http.Server{Handler: http.HandlerFunc(mainHandler)}

	return server.Serve(tlsListener)
}

//Stolen from: https://github.com/drk1wi/Modlishka/blob/00a2385a0952c48202ed0e314b0be016e0613ba7/core/proxy.go#L375

func decompress(httpResponse *http.Response) (buffer []byte, err error) {

	body := httpResponse.Body
	compression := httpResponse.Header.Get("Content-Encoding")

	var reader io.ReadCloser

	switch compression {
	case "x-gzip":
		fallthrough
	case "gzip":
		// A format using the Lempel-Ziv coding (LZ77), with a 32-bit CRC.

		reader, err = gzip.NewReader(body)
		if err != io.EOF {
			buffer, _ = io.ReadAll(reader)
			defer reader.Close()
		} else {
			// Unset error
			err = nil
		}

	case "deflate":
		// Using the zlib structure (defined in RFC 1950) with the deflate compression algorithm (defined in RFC 1951).

		reader = flate.NewReader(body)
		buffer, _ = io.ReadAll(reader)
		defer reader.Close()

	case "br":
		// A format using the Brotli algorithm.

		c := brotli.ReaderConfig{}
		reader, err = brotli.NewReader(body, &c)
		buffer, _ = io.ReadAll(reader)
		defer reader.Close()

	case "compress":
		// Unhandled: Fallback to default

		fallthrough

	default:
		reader = body
		buffer, err = io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	}

	return
}

//GZIP content
func gzipBuffer(input []byte) []byte {

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(input); err != nil {
		panic(err)
	}
	if err := gz.Flush(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

//Deflate content
func deflateBuffer(input []byte) []byte {

	var b bytes.Buffer
	zz, err := flate.NewWriter(&b, 0)

	if err != nil {
		panic(err)
	}
	if _, err = zz.Write(input); err != nil {
		panic(err)
	}
	if err := zz.Flush(); err != nil {
		panic(err)
	}
	if err := zz.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func compress(httpResponse *http.Response, buffer []byte) {

	compression := httpResponse.Header.Get("Content-Encoding")
	switch compression {
	case "x-gzip":
		fallthrough
	case "gzip":
		buffer = gzipBuffer(buffer)

	case "deflate":
		buffer = deflateBuffer(buffer)

	case "br":
		// Brotli writer is not available just compress with something else
		httpResponse.Header.Set("Content-Encoding", "deflate")
		buffer = deflateBuffer(buffer)

	default:
		// Whatif?
	}

	body := io.NopCloser(bytes.NewReader(buffer))
	httpResponse.Body = body
	httpResponse.ContentLength = int64(len(buffer))
	httpResponse.Header.Set("Content-Length", strconv.Itoa(len(buffer)))

	httpResponse.Body.Close()
}
