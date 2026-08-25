package tlsautocert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureCertificate makes sure a self-signed TLS certificate exists at
// certPath/keyPath inside certDir. If the files don't exist or the
// certificate is expired/about to expire, a new one is generated.
//
// The certificate includes the machine's hostname and all non-loopback
// local IP addresses as SANs (Subject Alternative Names) so browsers
// can connect via https://hostname:port or https://<ip>:port without
// additional configuration (a warning will still appear for self-signed
// certs unless the user trusts the CA).
//
// Returns the cert and key paths, and whether they were (re)generated.
func EnsureCertificate(certDir string) (certPath, keyPath string, regenerated bool, err error) {
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return "", "", false, fmt.Errorf("create cert dir: %w", err)
	}

	certPath = filepath.Join(certDir, "cert.pem")
	keyPath = filepath.Join(certDir, "key.pem")

	// Check if existing cert is valid
	if valid, reason := isCertValid(certPath, keyPath); valid {
		log.Printf("[tls] existing certificate is valid (%s)", certPath)
		return certPath, keyPath, false, nil
	} else {
		log.Printf("[tls] regenerating certificate: %s", reason)
	}

	if err := generateCert(certPath, keyPath); err != nil {
		return "", "", false, fmt.Errorf("generate certificate: %w", err)
	}

	log.Printf("[tls] generated self-signed certificate: %s", certPath)
	return certPath, keyPath, true, nil
}

// isCertValid checks whether the existing cert+key files exist, can be
// parsed, and are not expired or about to expire (within 7 days).
func isCertValid(certPath, keyPath string) (valid bool, reason string) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return false, "cert file missing or unreadable"
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return false, "key file missing or unreadable"
	}

	// Parse cert
	block, _ := pem.Decode(certData)
	if block == nil {
		return false, "cert PEM decode failed"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Sprintf("cert parse: %v", err)
	}

	// Parse key
	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return false, "key PEM decode failed"
	}
	if _, err := x509.ParseECPrivateKey(keyBlock.Bytes); err != nil {
		if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
			return false, fmt.Sprintf("key parse: %v", err)
		}
	}

	// Check expiry — regenerate if within 7 days of expiry
	now := time.Now()
	if now.After(cert.NotAfter) {
		return false, "certificate expired"
	}
	if now.Add(7 * 24 * time.Hour).After(cert.NotAfter) {
		return false, "certificate expires within 7 days"
	}

	// Check that the cert covers the current hostname and IPs
	hostname := getHostname()
	localIPs := getLocalIPs()

	hasHostname := false
	for _, san := range cert.DNSNames {
		if san == hostname {
			hasHostname = true
			break
		}
	}

	hasAllIPs := true
	for _, ip := range localIPs {
		found := false
		for _, certIP := range cert.IPAddresses {
			if certIP.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			hasAllIPs = false
			break
		}
	}

	if !hasHostname {
		return false, fmt.Sprintf("certificate missing hostname %q", hostname)
	}
	if !hasAllIPs {
		return false, "certificate missing one or more local IP addresses"
	}

	return true, ""
}

// generateCert creates a new self-signed ECDSA certificate and key.
func generateCert(certPath, keyPath string) error {
	// Generate ECDSA private key (P-256)
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// Collect host info for SANs
	hostname := getHostname()
	localIPs := getLocalIPs()

	// Build SAN lists
	dnsNames := []string{hostname}
	// Also add "localhost" so https://localhost works too
	dnsNames = append(dnsNames, "localhost")

	ipAddrs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	ipAddrs = append(ipAddrs, localIPs...)

	// Serial number
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"OpenPolyPrint"},
			CommonName:   hostname,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	// Write cert PEM
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open cert file: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("encode cert PEM: %w", err)
	}

	// Write key PEM
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open key file: %w", err)
	}
	defer keyFile.Close()
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return fmt.Errorf("encode key PEM: %w", err)
	}

	return nil
}

// getHostname returns the machine's hostname, or "openpolyprint" if it
// can't be determined.
func getHostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "openpolyprint"
	}
	return name
}

// getLocalIPs returns all non-loopback IPv4 and IPv6 addresses on the
// machine's network interfaces.
func getLocalIPs() []net.IP {
	var ips []net.IP
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range interfaces {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			// Only include global unicast addresses
			if ipNet.IP.IsGlobalUnicast() {
				ips = append(ips, ipNet.IP)
			}
		}
	}
	return ips
}
