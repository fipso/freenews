package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

func parseQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			var proxy bool
			for _, proxyHost := range proxyHosts {
				if strings.Contains(q.Name, proxyHost) {
					proxy = true
					break
				}
			}
			if !proxy {
				continue
			}
			record := fmt.Sprintf("%s A %s", q.Name, *publicIP)
			rr, err := dns.NewRR(record)
			if err != nil {
				log.Println(err)
				continue
			}
			m.Answer = append(m.Answer, rr)
		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Compress = false

	switch req.Opcode {
	case dns.OpcodeQuery:
		parseQuery(m)
	}

	if len(m.Answer) == 0 {
		c := &dns.Client{Net: "udp"}
		res, _, err := c.Exchange(req, "1.1.1.1:53")
		if err != nil {
			dns.HandleFailed(w, req)
			log.Println("[ERR] ", err)
			return
		}
		w.WriteMsg(res)
		return
	}

	log.Printf("[DNS] Response: %s", m.Answer)
	w.WriteMsg(m)
}

func serveDNS() {
	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	// start server
	server := &dns.Server{Addr: ":" + strconv.Itoa(*dnsPort), Net: "udp"}
	log.Printf("[DNS] Listening on 0.0.0.0:%d (udp only)", *dnsPort)
	err := server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		log.Fatalf("[ERR] Failed to start server: %s\n ", err.Error())
	}
}
