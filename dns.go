package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// answerQuery fills m with our own answers. It returns false if the query
// should be forwarded to the upstream DNS instead.
func answerQuery(m *dns.Msg) (handled bool) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			handled = answerQuestionType(m, q, "A", *publicIP, "127.0.0.1") || handled
		case dns.TypeAAAA:
			handled = answerQuestionType(m, q, "AAAA", *mitmAAAA, "::1") || handled
		}
	}
	return handled
}

func answerQuestionType(m *dns.Msg, q dns.Question, _type, mitmHost, blockHost string) bool {
	host := strings.ToLower(strings.TrimSuffix(q.Name, "."))

	// Check if question is for the info host or on unpaywall list
	if compareBase(host, config.InfoHost) || getHostOptions(host) != nil {
		// No address of this type configured (e.g. no -mitmAAAA): answer with
		// an empty NOERROR response. Forwarding upstream would hand out the real
		// address and the client would bypass the proxy over that protocol.
		if mitmHost == "" {
			return true
		}
		return appendRecord(m, q, _type, mitmHost)
	}

	if isBlocked(host) {
		return appendRecord(m, q, _type, blockHost)
	}
	return false
}

func appendRecord(m *dns.Msg, q dns.Question, _type, ip string) bool {
	rr, err := dns.NewRR(fmt.Sprintf("%s 300 %s %s", q.Name, _type, ip))
	if err != nil {
		log.Println("[ERR]", err)
		return false
	}
	m.Answer = append(m.Answer, rr)
	return true
}

func handleDnsRequest(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Compress = false

	handled := false
	switch req.Opcode {
	case dns.OpcodeQuery:
		handled = answerQuery(m)
	}

	// If we dont know what to do forward request to upstream dns
	if !handled {
		res, err := forwardUpstream(req)
		if err != nil {
			dns.HandleFailed(w, req)
			log.Println("[ERR]", err)
			return
		}
		w.WriteMsg(res)
		return
	}

	log.Printf("[DNS] Response: %s", m.Answer)
	w.WriteMsg(m)
}

func forwardUpstream(req *dns.Msg) (*dns.Msg, error) {
	res, _, err := (&dns.Client{Net: "udp"}).Exchange(req, config.UpstreamDNS)
	// Large responses don't fit in UDP. Retry over TCP
	if err == nil && res.Truncated {
		res, _, err = (&dns.Client{Net: "tcp"}).Exchange(req, config.UpstreamDNS)
	}
	return res, err
}

func serveDNS() {
	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	for _, network := range []string{"udp", "tcp"} {
		go func(network string) {
			log.Printf("[DNS] Listening on 0.0.0.0:%d(%s)", *dnsPort, network)
			server := &dns.Server{Addr: ":" + strconv.Itoa(*dnsPort), Net: network}
			if err := server.ListenAndServe(); err != nil {
				log.Fatalf("[ERR] Failed to start DNS server (%s): %s\n ", network, err.Error())
			}
		}(network)
	}
}

func serveDNSoverTLS() error {
	log.Printf("[DNS-TLS] Listening on %s:%d(tcp/tls)", *dotDomain, *dnsTlsPort)
	server := &dns.Server{
		Addr:      ":" + strconv.Itoa(*dnsTlsPort),
		Net:       "tcp-tls",
		TLSConfig: tlsDoTServerConfig,
	}
	return server.ListenAndServe()
}
