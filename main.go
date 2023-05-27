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
var blockListPath *string
var config Config

func main() {
	//Parse flags
	publicIP = flag.String("publicIP", getPublicIP(), "public interface ip address")
	dnsPort = flag.Int("dnsPort", 53, "port")
	dnsTlsPort = flag.Int("dnsTlsPort", 853, "port")
	dotDomain = flag.String("dotDomain", "", "domain for DNS over TLS")
	blockListPath = flag.String("blockList", "", "path to a DNS block list")
	flag.Parse()

	//Parse config file
	//TODO make flags overridable
	parseConfigFile()

	log.Printf("[*] Welcome. Public DNS Sever IP: %s", *publicIP)
	setupCerts()
	if len(ca.Signature) != 0 {
		log.Printf("[*] CA Signature: %x...", ca.Signature[:16])
	} else {
		log.Printf("[*] Generated New CA:\n%s ", caString)
	}

        if *blockListPath != "" {
		log.Printf("[*] Using block list: %s", *blockListPath)
		loadBlockList()
	}

	go serveDNS()
	if *dotDomain != "" {
		setupDoTCerts()
		go serveDNSoverTLS()
	}

	serveHTTP()
}
