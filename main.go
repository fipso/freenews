package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net/http"
)

var ca *x509.Certificate
var transport *http.Transport
var tlsHttpServerConfig *tls.Config
var tlsDoTServerConfig *tls.Config
var caString string
var publicIP *string
var dnsPort *int
var dnsTlsPort *int
var dotDomain *string
var config Config

func main() {
	//Parse flags
	publicIP = flag.String("publicIP", getPublicIP(), "public interface ip address")
	dnsPort = flag.Int("dnsPort", 53, "port")
	dnsTlsPort = flag.Int("dnsTlsPort", 853, "port")
	dotDomain = flag.String("dotDomain", "", "domain for DNS over TLS")
	flag.Parse()

	//Parse config file
	//TODO make flags overridable
	parseConfigFile()

	log.Printf("[*] Welcome. Public DNS Sever IP: %s", *publicIP)
	setupCerts()
	log.Printf("[*] CA Signature: %x...", ca.Signature[:16])

	go serveDNS()
	if *dotDomain != "" {
		setupDoTCerts()
		go serveDNSoverTLS()
	}

	serveHTTP()

}
