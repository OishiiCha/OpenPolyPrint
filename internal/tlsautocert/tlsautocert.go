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

// EnsureCertificate makes sure a TLS certificate exists at
// certPath/keyPath inside certDir, signed by a local CA (caCertPath/caKeyPath).
//
// The approach mirrors mkcert: a local root CA is generated once and used
// to sign the server certificate. The CA cert can be downloaded and
// installed into the system/browser trust store to eliminate browser
// security warnings.
//
// The server certificate includes the machine's hostname and all local
// IP addresses as SANs.
//
// Returns the server cert and key paths, the CA cert path, and whether
// they were (re)generated.
func EnsureCertificate(certDir string) (certPath, keyPath, caCertPath string, regenerated bool, err error) {
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return "", "", "", false, fmt.Errorf("create cert dir: %w", err)
	}

	certPath = filepath.Join(certDir, "cert.pem")
	keyPath = filepath.Join(certDir, "key.pem")
	caCertPath = filepath.Join(certDir, "ca.pem")
	caKeyPath := filepath.Join(certDir, "ca-key.pem")

	// Ensure the CA exists (generate once, reuse)
	caCert, caKey, caRegenerated, err := ensureCA(caCertPath, caKeyPath)
	if err != nil {
		return "", "", "", false, fmt.Errorf("ensure CA: %w", err)
	}

	// Check if existing server cert is valid
	if !caRegenerated {
		if valid, reason := isServerCertValid(certPath, keyPath, caCertPath); valid {
			log.Printf("[tls] existing certificate is valid (%s)", certPath)
			return certPath, keyPath, caCertPath, false, nil
		} else {
			log.Printf("[tls] regenerating server certificate: %s", reason)
		}
	} else {
		log.Printf("[tls] CA was regenerated, re-signing server certificate")
	}

	if err := generateServerCert(certPath, keyPath, caCert, caKey); err != nil {
		return "", "", "", false, fmt.Errorf("generate server certificate: %w", err)
	}

	log.Printf("[tls] generated server certificate: %s (signed by CA: %s)", certPath, caCertPath)
	return certPath, keyPath, caCertPath, true, nil
}

// ensureCA loads or generates the local root CA.
func ensureCA(caCertPath, caKeyPath string) (cert *x509.Certificate, key *ecdsa.PrivateKey, regenerated bool, err error) {
	// Try loading existing CA
	certData, certErr := os.ReadFile(caCertPath)
	keyData, keyErr := os.ReadFile(caKeyPath)

	if certErr == nil && keyErr == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			parsedCert, perr := x509.ParseCertificate(block.Bytes)
			if perr == nil {
				keyBlock, _ := pem.Decode(keyData)
				if keyBlock != nil {
					parsedKey, kerr := x509.ParseECPrivateKey(keyBlock.Bytes)
					if kerr == nil {
						// Check CA isn't expired
						if time.Now().Before(parsedCert.NotAfter) {
							log.Printf("[tls] using existing CA: %s (valid until %s)", caCertPath, parsedCert.NotAfter.Format("2006-01-02"))
							return parsedCert, parsedKey, false, nil
						}
						log.Printf("[tls] CA expired, regenerating")
					}
				}
			}
		}
	}

	// Generate new CA
	log.Printf("[tls] generating new root CA: %s", caCertPath)
	cert, key, err = generateCA()
	if err != nil {
		return nil, nil, false, err
	}

	// Write CA cert
	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, nil, false, fmt.Errorf("create CA certificate: %w", err)
	}
	if err := writePEM(caCertPath, "CERTIFICATE", certDER, 0o644); err != nil {
		return nil, nil, false, err
	}

	// Write CA key
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, false, fmt.Errorf("marshal CA key: %w", err)
	}
	if err := writePEM(caKeyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return nil, nil, false, err
	}

	return cert, key, true, nil
}

// generateCA creates a new root CA certificate and key.
func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"OpenPolyPrint"},
			CommonName:   "OpenPolyPrint Local CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Parse the template into a certificate (self-signed)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}

	return cert, privKey, nil
}

// isServerCertValid checks whether the existing server cert+key files are
// valid, not expired, signed by the current CA, and cover the current
// hostname and IPs.
func isServerCertValid(certPath, keyPath, caCertPath string) (valid bool, reason string) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return false, "server cert file missing"
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return false, "server key file missing"
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return false, "cert PEM decode failed"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Sprintf("cert parse: %v", err)
	}

	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return false, "key PEM decode failed"
	}
	if _, err := x509.ParseECPrivateKey(keyBlock.Bytes); err != nil {
		if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
			return false, fmt.Sprintf("key parse: %v", err)
		}
	}

	// Check expiry
	now := time.Now()
	if now.After(cert.NotAfter) {
		return false, "certificate expired"
	}
	if now.Add(7 * 24 * time.Hour).After(cert.NotAfter) {
		return false, "certificate expires within 7 days"
	}

	// Verify it was signed by the current CA
	caData, err := os.ReadFile(caCertPath)
	if err != nil {
		return false, "CA cert file missing"
	}
	caBlock, _ := pem.Decode(caData)
	if caBlock == nil {
		return false, "CA cert PEM decode failed"
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return false, fmt.Sprintf("CA cert parse: %v", err)
	}
	if err := cert.CheckSignatureFrom(caCert); err != nil {
		return false, "certificate not signed by current CA"
	}

	// Check hostname and IPs
	hostname := getHostname()
	localIPs := getLocalIPs()

	hasHostname := false
	for _, san := range cert.DNSNames {
		if san == hostname || san == "localhost" {
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

// generateServerCert creates a new server certificate signed by the CA.
func generateServerCert(certPath, keyPath string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}

	hostname := getHostname()
	localIPs := getLocalIPs()

	dnsNames := []string{hostname, "localhost"}
	ipAddrs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	ipAddrs = append(ipAddrs, localIPs...)

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
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create server certificate: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", certDER, 0o644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("marshal server key: %w", err)
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return err
	}

	return nil
}

// writePEM writes a PEM block to a file with the given permissions.
func writePEM(path, pemType string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: data})
}

// CACertPath returns the path to the CA certificate, or empty if not yet generated.
var cachedCACertPath string

func getHostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "openpolyprint"
	}
	return name
}

func getLocalIPs() []net.IP {
	var ips []net.IP
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range interfaces {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.IsGlobalUnicast() {
				ips = append(ips, ipNet.IP)
			}
		}
	}
	return ips
}
