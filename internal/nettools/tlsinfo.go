package nettools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// CertInfo describes a server's TLS certificate and connection.
type CertInfo struct {
	// Subject and Issuer are the leaf certificate's distinguished names.
	Subject, Issuer string
	// DNSNames lists the subject alternative names; Chain lists the subject
	// of every certificate presented, leaf first.
	DNSNames, Chain []string
	// NotBefore and NotAfter bound the leaf's validity period.
	NotBefore, NotAfter time.Time
	// DaysLeft is the number of whole days until NotAfter (negative when
	// expired).
	DaysLeft int
	// TLSVersion and Cipher describe the negotiated connection.
	TLSVersion, Cipher string
	// Valid is true when the chain verified against the system roots for
	// the host name; otherwise VerifyErr explains why.
	Valid     bool
	VerifyErr string
}

const tlsTimeout = 8 * time.Second

// InspectTLS connects to hostport (port 443 by default) and reports the
// certificate. If verification fails, it reconnects without verification so
// the certificate can still be shown, with Valid=false and VerifyErr set.
func InspectTLS(ctx context.Context, hostport string) (CertInfo, error) {
	var info CertInfo
	hp := strings.TrimSpace(hostport)
	if i := strings.Index(hp, "://"); i >= 0 {
		hp = hp[i+3:]
	}
	if i := strings.IndexAny(hp, "/?#"); i >= 0 {
		hp = hp[:i]
	}
	if hp == "" {
		return info, errors.New("tls: empty host")
	}
	host, port, err := net.SplitHostPort(hp)
	if err != nil {
		host, port = cleanHost(hp), "443"
	}
	addr := net.JoinHostPort(host, port)

	state, verr := tlsHandshake(ctx, addr, host, false)
	if verr != nil {
		var certErr *tls.CertificateVerificationError
		var unk x509.UnknownAuthorityError
		var hostErr x509.HostnameError
		var invalid x509.CertificateInvalidError
		isVerify := errors.As(verr, &certErr) || errors.As(verr, &unk) || errors.As(verr, &hostErr) || errors.As(verr, &invalid)
		if !isVerify {
			return info, verr
		}
		state, err = tlsHandshake(ctx, addr, host, true)
		if err != nil {
			return info, err
		}
		info.VerifyErr = verr.Error()
	} else {
		info.Valid = true
	}
	if len(state.PeerCertificates) == 0 {
		return info, errors.New("tls: server sent no certificate")
	}
	leaf := state.PeerCertificates[0]
	info.Subject = leaf.Subject.String()
	info.Issuer = leaf.Issuer.String()
	info.DNSNames = append([]string(nil), leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		info.DNSNames = append(info.DNSNames, ip.String())
	}
	for _, c := range state.PeerCertificates {
		info.Chain = append(info.Chain, c.Subject.String())
	}
	info.NotBefore, info.NotAfter = leaf.NotBefore, leaf.NotAfter
	info.DaysLeft = int(math.Floor(time.Until(leaf.NotAfter).Hours() / 24))
	info.TLSVersion = tls.VersionName(state.Version)
	info.Cipher = tls.CipherSuiteName(state.CipherSuite)
	return info, nil
}

// tlsHandshake dials addr and completes a TLS handshake.
func tlsHandshake(ctx context.Context, addr, serverName string, insecure bool) (tls.ConnectionState, error) {
	ctx, cancel := context.WithTimeout(ctx, tlsTimeout)
	defer cancel()
	cfg := &tls.Config{ServerName: serverName}
	if insecure {
		cfg.InsecureSkipVerify = true //nolint:gosec // only to display an invalid certificate
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: cfg}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return tls.ConnectionState{}, fmt.Errorf("tls %s: %w", addr, err)
	}
	tc, ok := c.(*tls.Conn)
	if !ok {
		_ = c.Close()
		return tls.ConnectionState{}, errors.New("tls: unexpected connection type")
	}
	state := tc.ConnectionState()
	// The handshake already succeeded; a close error does not change what
	// we learned about the certificate.
	_ = tc.Close()
	return state, nil
}
