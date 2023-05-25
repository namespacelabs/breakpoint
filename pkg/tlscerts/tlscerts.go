package tlscerts

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

type Subjects struct {
	DNSNames    []string
	IPAddresses []net.IP
}

func GenerateECDSAPair(subjects Subjects, duration time.Duration) ([]byte, []byte, error) {
	serial, err := newSerialNumber()
	if err != nil {
		return nil, nil, err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	privDer, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}

	privPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDer})

	template := &x509.Certificate{
		SerialNumber: serial,
		NotAfter:     time.Now().Add(duration),
		DNSNames:     subjects.DNSNames,
		IPAddresses:  subjects.IPAddresses,
	}

	certDer, err := x509.CreateCertificate(rand.Reader, template, template, priv.Public(), priv)
	if err != nil {
		return nil, nil, err
	}

	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})
	if err != nil {
		return nil, nil, err
	}

	return certPem, privPem, nil
}

func newSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}
