package utils

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diniboy1123/manifesto/config"
)

var proxyClient *http.Client

// GetProxyClient returns an HTTP client that respects the configured proxy settings.
// It sets up http and https proxies, and honors the no_proxy rules if configured.
func GetProxyClient() *http.Client {
	if proxyClient != nil {
		return proxyClient
	}

	cfg := config.Get()

	proxyFunc := http.ProxyFromEnvironment // fallback

	if cfg.HttpProxy != "" || cfg.HttpsProxy != "" {
		proxyFunc = func(req *http.Request) (*url.URL, error) {
			host := req.URL.Hostname()
			scheme := req.URL.Scheme

			if cfg.NoProxy != "" && shouldBypassProxy(host, cfg.NoProxy) {
				return nil, nil
			}
			if scheme == "http" && cfg.HttpProxy != "" {
				return url.Parse(cfg.HttpProxy)
			}
			if scheme == "https" && cfg.HttpsProxy != "" {
				return url.Parse(cfg.HttpsProxy)
			}
			return nil, nil
		}
	}

	tlsConfig := &tls.Config{}

	if cfg.TlsClientInsecure {
		tlsConfig.InsecureSkipVerify = true
	}

	transport := &http.Transport{
		Proxy:                 proxyFunc,
		TLSClientConfig:       tlsConfig,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxyClient = &http.Client{Transport: transport}
	return proxyClient
}

// shouldBypassProxy checks if the given host should bypass the proxy based on the no_proxy settings.
// It supports wildcard matching and domain suffix matching.
// For example, if no_proxy is set to "*.example.com", it will bypass the proxy for "test.example.com".
// If no_proxy is set to "*", it will bypass the proxy for all hosts.
// It also trims whitespace and ignores empty entries in the no_proxy list.
// The function returns true if the proxy should be bypassed for the given host, false otherwise.
func shouldBypassProxy(host, noProxy string) bool {
	noProxyList := filepath.SplitList(noProxy)
	for _, np := range noProxyList {
		np = strings.TrimSpace(np)
		if np == "" {
			continue
		}
		if np == "*" || host == np || (strings.HasPrefix(np, ".") && strings.HasSuffix(host, np)) {
			return true
		}
	}
	return false
}
