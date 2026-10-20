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
	publicIP = flag.String("publicIP", "", "Public interface ip address (auto-detected if empty)")
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

	if *publicIP == "" {
		ip, err := getPublicIP()
		if err != nil {
			log.Fatalf("[ERR] Could not detect public IP (%s). Set it with -publicIP", err)
		}
		*publicIP = ip
	}

	serverIps := make([]string, 0, 2)
	serverIps = append(serverIps, *publicIP)
	if *mitmAAAA != "" {
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
		go func() {
			if err := serveDNSoverTLS(); err != nil {
				log.Fatalf("[ERR] Failed to start DNS over TLS server: %s", err)
			}
		}()
	}

	serveHTTP()
}
