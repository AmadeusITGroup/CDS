package cdstls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Build appropriate *tls.Config based on tcfg content
func SetupTLSConfig(tcfg TLSConfig) (*tls.Config, error) {

	tlsConfig := &tls.Config{}
	if tcfg.CertFile != "" && tcfg.KeyFile != "" {
		var err error
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(tcfg.CertFile, tcfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to load TLS certificate using key pair: %q, %q",
				tcfg.CertFile,
				tcfg.KeyFile,
			)
		}
	}

	if tcfg.CAFile != "" {
		b, err := os.ReadFile(tcfg.CAFile)
		if err != nil {
			return nil, err
		}
		ca := x509.NewCertPool()
		ok := ca.AppendCertsFromPEM([]byte(b))
		if !ok {
			return nil, fmt.Errorf(
				"failed to parse root certificate: %q",
				tcfg.CAFile,
			)
		}
		if tcfg.Server {
			tlsConfig.ClientCAs = ca
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.RootCAs = ca
		}
		tlsConfig.ServerName = tcfg.ServerAddress
	}
	return tlsConfig, nil
}

// Define the parameters that SetupTLSConfig uses to determine what type of *tls.Config to return.
type TLSConfig struct {
	CertFile      string
	KeyFile       string
	CAFile        string
	ServerAddress string
	Server        bool
}
