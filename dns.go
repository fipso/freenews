package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/miekg/dns"
)

func answerQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			host := q.Name[:len(q.Name)-1]

			// Check if question is for the info host or on unpaywall list
			options := getHostOptions(host)
			if compareBase(host, config.InfoHost) || options != nil {
				record := fmt.Sprintf("%s A %s", q.Name, *publicIP)
				rr, err := dns.NewRR(record)
				if err != nil {
					log.Println("[ERR]", err)
					continue
				}
				m.Answer = append(m.Answer, rr)
				continue
			}

			// Check if host is on blocklist
			for _, blocked := range blockList {
				if host == blocked {
					record := fmt.Sprintf("%s A 127.0.0.1", q.Name)
					rr, err := dns.NewRR(record)
					if err != nil {
						log.Println("[ERR]", err)
						continue
					}
					m.Answer = append(m.Answer, rr)
					break
				}
			}

		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Compress = false

	switch req.Opcode {
	case dns.OpcodeQuery:
		answerQuery(m)
	}

	// If we dont know what to do forward request to upstream dns
	if len(m.Answer) == 0 {
		c := &dns.Client{Net: "udp"}
		res, _, err := c.Exchange(req, config.UpstreamDNS)
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

func serveDNS() {
	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	// start server
	go func() {
		log.Printf("[DNS] Listening on 0.0.0.0:%d(udp only)", *dnsPort)
		server := &dns.Server{Addr: ":" + strconv.Itoa(*dnsPort), Net: "udp"}
		err := server.ListenAndServe()
		defer server.Shutdown()
		if err != nil {
			log.Fatalf("[ERR] Failed to start DNS server: %s\n ", err.Error())
		}
	}()
}

func serveDNSoverTLS() error {
	log.Printf("[DNS-TLS] Listening on %s:%d(tcp/tls)", *dotDomain, *dnsTlsPort)
	server := &dns.Server{
		Addr:      ":" + strconv.Itoa(*dnsTlsPort),
		Net:       "tcp-tls",
		TLSConfig: tlsDoTServerConfig,
	}
	err := server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		return err
	}
	return nil
}
