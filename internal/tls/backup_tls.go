package cdstls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

func BuildAgentServerCerts() error {

	caPrivateKey, caCert, caErr := createCertificateAuthority()
	if caErr != nil {
		return fmt.Errorf("failed to build agent server TLS certificates: %v", caErr)
	}
	serverPrivateKey, serverCert, srvCertErr := createServerCerts(caPrivateKey, caCert)
	if srvCertErr != nil {
		return fmt.Errorf("failed to build agent server TLS certificates: %v", srvCertErr)
	}
	serverPrivateKeyDER, marshalErr := x509.MarshalECPrivateKey(serverPrivateKey)
	if marshalErr != nil {
		return fmt.Errorf("failed to build agent server TLS certificates: %v", marshalErr)
	}

	writeToFile("ca_cert.pem", caCert)
	writeToFile("server_cert.pem", serverCert)
	writeToFile("server_key.pem", pemEncode("EC PRIVATE KEY", serverPrivateKeyDER))
	return nil
}

// Create a self-signed CA's private key and certificate
func createCertificateAuthority() (*ecdsa.PrivateKey, []byte, error) {
	caPrivateKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyErr != nil {
		return nil, nil, fmt.Errorf("failed to generate CA private key: %v", keyErr)
	}

	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"My CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, caErr := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivateKey.PublicKey, caPrivateKey)
	if caErr != nil {
		return nil, nil, fmt.Errorf("failed to generate CA cert: %v", caErr)
	}

	return caPrivateKey, pemEncode("CERTIFICATE", caCertDER), nil
}

// Create server private key and certificate
func createServerCerts(caPrivateKey *ecdsa.PrivateKey, caCertPEM []byte) (*ecdsa.PrivateKey, []byte, error) {

	// Create a server private key and Certificate Signing Request
	serverPrivateKey, keySrvErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keySrvErr != nil {
		return nil, nil, fmt.Errorf("failed to generate server PrivateKey: %v", keySrvErr)
	}

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"My Server"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	csrDER, crErr := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, serverPrivateKey)
	if crErr != nil {
		return nil, nil, fmt.Errorf("failed to generate CSR: %v", crErr)
	}
	csr, parseErr := x509.ParseCertificateRequest(csrDER)
	if parseErr != nil {
		return nil, nil, fmt.Errorf("failed to parse CSR: %v", parseErr)
	}

	serverCertTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"My Server"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	caCert, _ := pemDecode(caCertPEM)

	// Sign the server CSR with the CA private key to get the server certificate
	serverCertDER, serverCertErr := x509.CreateCertificate(rand.Reader, &serverCertTemplate, caCert, csr.PublicKey, caPrivateKey)
	if serverCertErr != nil {
		return nil, nil, fmt.Errorf("failed to generate server cert: %v", serverCertErr)
	}

	return serverPrivateKey, pemEncode("CERTIFICATE", serverCertDER), nil

}

func pemEncode(typ string, der []byte) []byte {
	block := &pem.Block{
		Type:  typ,
		Bytes: der,
	}
	pemData := pem.EncodeToMemory(block)
	return pemData
}

func pemDecode(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	cert, err := x509.ParseCertificate(block.Bytes)
	return cert, err
}

func writeToFile(filename string, data []byte) {
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = file.Write(data)
	if err != nil {
		panic(err)
	}
}
