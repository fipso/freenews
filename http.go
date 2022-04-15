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
	if r.Host == "freenews.xxx" {
		w.Write([]byte(fmt.Sprintf(
			"Welcome to freenews.xxx\nPlease make sure to install the following CA:\n\n%s\n\nCurrently unlocking:\n%s",
			caString, strings.Join(proxyHosts[1:], "\n"))))
		return
	}

	url, err := url.Parse("https://" + r.Host)
	if err != nil {
		log.Fatalf("%s . Terminating.", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = transport
	director := proxy.Director
	proxy.Director = func(req *http.Request){
		//spoof twitter referer
		req.Header.Set("Referer", "https://t.co/")
		//spoof google bot ua and ip
		req.Header.Set("User-Agent", "AdsBot-Google (+http://www.google.com/adsbot.html)")
		req.Header.Set("X-Forwarded-For", "66.102.0.0")
		//disable cookies
		req.Header.Set("Cookie", "")
		req.Header.Set("Set-Cookie", "")
		director(req)
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		contentType := res.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			return nil
		}
		b, err := decompress(res)
		if err != nil {
			return  err
		}
		b = bytes.Replace(b, []byte("<body>"), []byte("<body><span style=\"background: #444444; font-family: Arial; width: 100% !important; display: block; color: white;\">Served by Freenews.xxx</span>"), -1) // replace html
		compress(res, b)
		log.Printf("Successfully Injected %s", res.Request.URL)
		return nil

	}
	proxy.ServeHTTP(w, r)
}

func serveHTTP() {
	//create custom resolver that does not use our own dns
	dnsResolverIP := "8.8.8.8:53"
	dnsResolverProto := "udp"
	dnsResolverTimeoutMs := 5000
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
			log.Println("dialing", addr)
			return dialer.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	go func() {
		server := &http.Server{Addr: "0.0.0.0:80", Handler: http.HandlerFunc(mainHandler)}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("%s . Terminating.", err)
		}
	}()

	if err := listenAndServeTLS(); err != nil {
		log.Fatalf("%s . Terminating.", err)
	}

}

func listenAndServeTLS() error {
	conn, err := net.Listen("tcp", "0.0.0.0:443")
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(conn, tlsServerConfig)
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
		// This is the original format of the UNIX gzip program.
		// The HTTP/1.1 standard also recommends that the servers supporting this content-encoding should recognize
		// x-gzip as an alias, for compatibility purposes.

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

		// A format using the Lempel-Ziv-Welch (LZW) algorithm.
		// The value name was taken from the UNIX compress program, which implemented this algorithm.
		// Like the compress program, which has disappeared from most UNIX distributions,
		// this content-encoding is not used by many browsers today, partly because of a patent issue (it expired in 2003).
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
