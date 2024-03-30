package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net/http"
	"strings"
)

var (
	ca                  *x509.Certificate
	transport           *http.Transport
	tlsHttpServerConfig *tls.Config
	tlsDoTServerConfig  *tls.Config
	caString            string
	publicIP            *string
	mitmAAAA            = flag.String("mitmAAAA", "", "IPv6 address to use for MITM")
	dnsPort             *int
	dnsTlsPort          *int
	httpPort            *int
	httpsPort           *int
	dotDomain           *string
	blockListPath       *string
	config              Config
)

func main() {
	//Parse flags
	publicIP = flag.String("publicIP", getPublicIP(), "Public interface ip address")
	dnsPort = flag.Int("dnsPort", 53, "Port for normal UDP DNS")
	dnsTlsPort = flag.Int("dnsTlsPort", 853, "Port for DNS over TLS aka. DoT")
	httpPort = flag.Int("httpPort", 80, "Port for HTTP Reverse Proxy")
	httpsPort = flag.Int("httpsPort", 443, "Port for HTTPS Reverse Proxy")
	dotDomain = flag.String("dotDomain", "", "Domain for DNS over TLS")
	blockListPath = flag.String("blockList", "", "Path to a DNS block list file")
	flag.Parse()

	//Parse config file
	//TODO make flags overridable
	parseConfigFile()

	serverIps := make([]string, 0, 2)
	if publicIP != nil {
		serverIps = append(serverIps, *publicIP)
	}
	if mitmAAAA != nil {
		serverIps = append(serverIps, *mitmAAAA)
	}
	log.Printf("[*] Welcome. Public DNS Server IPs: %v", strings.Join(serverIps, ", "))
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
