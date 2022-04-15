package main

import (
	"crypto/tls"
	"net/http"
)

var transport *http.Transport
var tlsServerConfig *tls.Config
var caString string

var proxyHosts = []string{
	//our own fake domain
	"freenews.xxx",
	//some big news pages that use paywalls
	"ft.com",
	"theguardian.com",
	"faz.net",
	"wsj.com",
	"nytimes.com",
}

func main() {
	setupCerts()
	go serveHTTP()
	serveDNS()
}
