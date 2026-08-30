package fixture

import (
	"crypto/tls"
	"net/http"
)

func permissive() *http.Transport {
	// ruleid: go-tls-skip-verify
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
}

func permissiveAmongOthers() *tls.Config {
	// ruleid: go-tls-skip-verify
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}
}

func permissiveLater() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	// ruleid: go-tls-skip-verify
	cfg.InsecureSkipVerify = true
	return cfg
}

func configurable(skip bool) *tls.Config {
	// ok: go-tls-skip-verify
	return &tls.Config{InsecureSkipVerify: skip}
}

func strict() *tls.Config {
	// ok: go-tls-skip-verify
	return &tls.Config{MinVersion: tls.VersionTLS13}
}
