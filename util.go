package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/bobesa/go-domain-util/domainutil"
	"github.com/dsnet/compress/brotli"
)

// https://stackoverflow.com/questions/41670155/get-public-ip-in-golang
type IP struct {
	Query string
}

func getPublicIP() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := client.Get("http://ip-api.com/json/")
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	var ip IP
	if err := json.Unmarshal(body, &ip); err != nil {
		return "", err
	}
	if net.ParseIP(ip.Query) == nil {
		return "", fmt.Errorf("unexpected response from ip-api.com: %q", body)
	}

	return ip.Query, nil
}

func compareBase(name1, name2 string) bool {
	base := domainutil.Domain(name1)
	return base != "" && base == domainutil.Domain(name2)
}

func getHostOptions(host string) *HostOptions {
	hostsMu.RLock()
	defer hostsMu.RUnlock()
	for _, entry := range config.Hosts {
		//Only compare domain + tld. Ignore subdomains
		if compareBase(entry.Name, host) {
			entry := entry
			return &entry
		}
	}
	return nil
}

//Stolen from: https://github.com/drk1wi/Modlishka/blob/00a2385a0952c48202ed0e314b0be016e0613ba7/core/proxy.go#L375

func decompress(httpResponse *http.Response) (buffer []byte, err error) {
	body := httpResponse.Body
	defer body.Close()
	compression := httpResponse.Header.Get("Content-Encoding")

	var reader io.ReadCloser

	switch compression {
	case "x-gzip":
		fallthrough
	case "gzip":
		// A format using the Lempel-Ziv coding (LZ77), with a 32-bit CRC.

		reader, err = gzip.NewReader(body)
		if err == io.EOF {
			// Empty body
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)

	case "deflate":
		// Using the zlib structure (defined in RFC 1950) with the deflate compression algorithm (defined in RFC 1951).

		reader = flate.NewReader(body)
		defer reader.Close()
		return io.ReadAll(reader)

	case "br":
		// A format using the Brotli algorithm.

		c := brotli.ReaderConfig{}
		reader, err = brotli.NewReader(body, &c)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)

	default:
		// Identity. Check canDecode before calling this for anything else
		return io.ReadAll(body)
	}
}

func canDecode(httpResponse *http.Response) bool {
	switch httpResponse.Header.Get("Content-Encoding") {
	case "", "identity", "gzip", "x-gzip", "deflate", "br":
		return true
	}
	return false
}

// setBody replaces the response body with an uncompressed buffer. The client
// talks to our own proxy, so compressing again isn't worth it.
func setBody(httpResponse *http.Response, buffer []byte) {
	httpResponse.Header.Del("Content-Encoding")
	httpResponse.Body = io.NopCloser(bytes.NewReader(buffer))
	httpResponse.ContentLength = int64(len(buffer))
	httpResponse.Header.Set("Content-Length", strconv.Itoa(len(buffer)))
}
