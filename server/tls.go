package server

import (
	"crypto/tls"
	"log"
	"net/http"
	"strings"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
)

func getTLSConfig(certMap []config.TLSDomainConfig, bogusDomain string) *tls.Config {
	certificates := map[string]tls.Certificate{}

	for _, entry := range certMap {
		cert, err := tls.LoadX509KeyPair(entry.Cert, entry.Key)
		if err != nil {
			log.Fatalf("Failed to load TLS certificate for %s: %v", entry.Domain, err)
		}
		certificates[entry.Domain] = cert
	}

	bogusCert := utils.GenerateSelfSignedCert(bogusDomain)

	return &tls.Config{
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			domain := strings.ToLower(clientHello.ServerName)
			if cert, exists := certificates[domain]; exists {
				return &cert, nil
			}

			return &bogusCert, nil
		},
		MinVersion: tls.VersionTLS12,
	}
}

func startHTTPSListener(srv *http.Server) {
	cfg := config.Get()
	tlsCfg := getTLSConfig(cfg.TLSDomainMap, cfg.BogusDomain)

	listener, err := tls.Listen("tcp", srv.Addr, tlsCfg)
	if err != nil {
		log.Fatalf("Failed to start HTTPS listener: %v", err)
	}

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTPS server error: %v", err)
	}
}
