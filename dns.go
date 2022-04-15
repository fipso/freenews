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
			log.Printf("Query for %s\n", q.Name)
			var proxy bool
			for _, proxyHost := range proxyHosts {
				if strings.HasSuffix(q.Name, fmt.Sprintf("%s.", proxyHost)) {
					proxy = true
					break
				}
			}
			if !proxy {
				continue
			}
			//TODO use public interface ip
			ip := "127.0.0.1"
			rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
			if err == nil {
				m.Answer = append(m.Answer, rr)
			}
		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	switch r.Opcode {
	case dns.OpcodeQuery:
		parseQuery(m)
	}

	//fail all domains we dont know
	//todo forward to 1.1.1.1
	if len(m.Answer) == 0 {
		dns.HandleFailed(w, r)
	} else {
		log.Println(m.Answer)
	}

	w.WriteMsg(m)
}

func serveDNS() {
	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	// start server
	port := 5300
	server := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
	log.Printf("Starting at %d\n", port)
	err := server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		log.Fatalf("Failed to start server: %s\n ", err.Error())
	}
}
