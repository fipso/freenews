package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	caPrivKey  *rsa.PrivateKey
	leafCerts  = map[string]*tls.Certificate{}
	leafCertMu sync.Mutex
)

func setupCerts() {
	if _, err := os.Stat("./cert"); err == nil {
		loadCA()
	} else if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir("./cert", os.ModePerm); err != nil {
			log.Fatal(err)
		}
		generateCA()
	} else {
		log.Fatal(err)
	}

	// Leaf certs are issued on demand per SNI name, so hosts added at runtime
	// via /addhost work without restarting
	tlsHttpServerConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"http/1.1"},
		GetCertificate: getLeafCert,
	}
}

func loadCA() {
	caPEMBytes, err := os.ReadFile("cert/ca.pem")
	if err != nil {
		log.Fatal(err)
	}
	caString = string(caPEMBytes)
	caPEM, _ := pem.Decode(caPEMBytes)
	if caPEM == nil {
		log.Fatal("cert/ca.pem: no PEM data found")
	}
	if ca, err = x509.ParseCertificate(caPEM.Bytes); err != nil {
		log.Fatal("cert/ca.pem: ", err)
	}
	if time.Now().After(ca.NotAfter) {
		log.Printf("[WARN] CA expired on %s. Delete cert/ca.pem and cert/key.pem to generate a new one", ca.NotAfter)
	}

	caPrivKeyPEMBytes, err := os.ReadFile("cert/key.pem")
	if err != nil {
		log.Fatal(err)
	}
	caPrivKeyPEM, _ := pem.Decode(caPrivKeyPEMBytes)
	if caPrivKeyPEM == nil {
		log.Fatal("cert/key.pem: no PEM data found")
	}
	if caPrivKey, err = x509.ParsePKCS1PrivateKey(caPrivKeyPEM.Bytes); err != nil {
		log.Fatal("cert/key.pem: ", err)
	}
}

func generateCA() {
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject: pkix.Name{
			Organization: []string{"FreeNews MITM CA"},
			CommonName:   "FreeNews MITM CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	var err error
	caPrivKey, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Fatal(err)
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, template, template, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Fatal(err)
	}
	if ca, err = x509.ParseCertificate(caBytes); err != nil {
		log.Fatal(err)
	}

	// pem encode
	caString = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes}))
	if err = os.WriteFile("cert/ca.pem", []byte(caString), 0600); err != nil {
		log.Fatal(err)
	}

	caPrivKeyString := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
	if err = os.WriteFile("cert/key.pem", caPrivKeyString, 0600); err != nil {
		log.Fatal(err)
	}
}

func getLeafCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(hello.ServerName)
	if name == "" {
		name = config.InfoHost
	}
	if !compareBase(name, config.InfoHost) && getHostOptions(name) == nil {
		return nil, fmt.Errorf("refusing to issue cert for %q: not on unpaywall list", name)
	}

	leafCertMu.Lock()
	defer leafCertMu.Unlock()
	if cert, ok := leafCerts[name]; ok && time.Now().Before(cert.Leaf.NotAfter.Add(-24*time.Hour)) {
		return cert, nil
	}
	cert, err := issueLeafCert(name)
	if err != nil {
		return nil, err
	}
	leafCerts[name] = cert
	return cert, nil
}

func issueLeafCert(name string) (*tls.Certificate, error) {
	// Apple rejects leaf certs valid for more than 825 days, browsers like
	// around a year. Never outlive the CA
	notAfter := time.Now().AddDate(1, 0, 0)
	if notAfter.After(ca.NotAfter) {
		notAfter = ca.NotAfter
	}
	template := &x509.Certificate{
		// Serials must be unique per issuer, or Firefox rejects the cert
		// after a restart
		SerialNumber: randomSerial(),
		Subject: pkix.Name{
			Organization: []string{"FreeNews MITM"},
			CommonName:   name,
		},
		DNSNames:    []string{name},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    notAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// ECDSA keys are generated in microseconds, unlike 4096 bit RSA
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

func randomSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		log.Fatal(err)
	}
	return serial
}

func setupDoTCerts() {
	if _, err := os.Stat("cert/dot_cert.pem"); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat("cert/dot_key.pem"); err != nil {
		log.Fatal(err)
	}

	// pem decode
	certPEMBytes, err := os.ReadFile("cert/dot_cert.pem")
	if err != nil {
		log.Fatal(err)
	}
	certPrivKeyPEMBytes, err := os.ReadFile("cert/dot_key.pem")
	if err != nil {
		log.Fatal(err)
	}

	serverCert, err := tls.X509KeyPair(certPEMBytes, certPrivKeyPEMBytes)
	if err != nil {
		log.Fatal(err)
	}
	//log.Printf("[*] DoT Cert %x... Expiry: %s", serverCert.Leaf.Signature[:16], serverCert.Leaf.NotAfter)

	tlsDoTServerConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
	}

}
