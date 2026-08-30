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
	"os/exec"
	"path/filepath"
	"runtime"
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

// CheckAndRegenerateIfNeeded checks if the existing certificate covers all
// current local IP addresses (including newly appeared ones like Tailscale,
// VPN, or DHCP changes). If IPs are missing, it regenerates the certificate.
// Returns true if the certificate was regenerated.
func CheckAndRegenerateIfNeeded(certDir string) (bool, error) {
	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")
	caCertPath := filepath.Join(certDir, "ca.pem")
	caKeyPath := filepath.Join(certDir, "ca-key.pem")

	// Check if existing cert is still valid (covers all current IPs)
	valid, reason := isServerCertValid(certPath, keyPath, caCertPath)
	if valid {
		return false, nil
	}

	log.Printf("[tls] certificate needs regeneration: %s", reason)

	// Re-sign the server certificate with the existing CA
	caCert, caKey, _, err := ensureCA(caCertPath, caKeyPath)
	if err != nil {
		return false, fmt.Errorf("ensure CA: %w", err)
	}
	if err := generateServerCert(certPath, keyPath, caCert, caKey); err != nil {
		return false, fmt.Errorf("generate server certificate: %w", err)
	}

	log.Printf("[tls] certificate regenerated with updated IP addresses")
	return true, nil
}

// InstallCAToSystemStore installs the CA certificate into the local
// system's trust store so that tools like curl, wget, and (on Linux)
// browsers using the system store will trust the HTTPS certificate
// without warnings.
//
// This installs on the HOST where the app is running (e.g. the Pi).
// Client devices need to use the installer scripts served by the app.
func InstallCAToSystemStore(caCertPath string) error {
	switch runtime.GOOS {
	case "linux":
		return installCALinux(caCertPath)
	case "darwin":
		return installCADarwin(caCertPath)
	case "windows":
		return installCAWindows(caCertPath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installCALinux(caCertPath string) error {
	// Debian/Ubuntu: /usr/local/share/ca-certificates/ + update-ca-certificates
	// Alpine: /usr/local/share/ca-certificates/ + update-ca-certificates
	// RHEL/Fedora: /etc/pki/ca-trust/source/anchors/ + update-ca-trust
	destDir := "/usr/local/share/ca-certificates"
	cmdName := "update-ca-certificates"

	// Check if update-ca-trust exists (RHEL/Fedora)
	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		destDir = "/etc/pki/ca-trust/source/anchors"
		cmdName = "update-ca-trust"
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create CA dir: %w", err)
	}

	// Copy the CA cert (must have .crt extension on Debian)
	dest := filepath.Join(destDir, "openpolyprint-ca.crt")
	data, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("read CA cert: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("write CA cert to %s: %w", dest, err)
	}

	// Run the update command
	cmd := exec.Command(cmdName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmdName, err)
	}

	log.Printf("[tls] CA installed to system trust store: %s", dest)
	return nil
}

func installCADarwin(caCertPath string) error {
	// macOS: add to System keychain
	cmd := exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain", caCertPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("security add-trusted-cert: %w", err)
	}
	log.Printf("[tls] CA installed to macOS System keychain")
	return nil
}

func installCAWindows(caCertPath string) error {
	// Windows: certutil -addstore Root <path>
	cmd := exec.Command("certutil", "-addstore", "Root", caCertPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("certutil -addstore Root: %w", err)
	}
	log.Printf("[tls] CA installed to Windows Trusted Root store")
	return nil
}

// IsCAInstalledInSystemStore checks whether the CA is already in the
// system trust store (best-effort, returns false if not sure).
func IsCAInstalledInSystemStore() bool {
	switch runtime.GOOS {
	case "linux":
		// Check if the file exists in the common locations
		candidates := []string{
			"/usr/local/share/ca-certificates/openpolyprint-ca.crt",
			"/etc/pki/ca-trust/source/anchors/openpolyprint-ca.crt",
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
		return false
	case "darwin":
		// Check if the cert is in the keychain (best-effort)
		cmd := exec.Command("security", "find-certificate", "-c", "OpenPolyPrint Local CA", "/Library/Keychains/System.keychain")
		return cmd.Run() == nil
	case "windows":
		cmd := exec.Command("certutil", "-verify", "OpenPolyPrint Local CA")
		return cmd.Run() == nil
	}
	return false
}

// WindowsInstallScript returns a .bat script that downloads the CA cert
// from the server and installs it into the Windows Trusted Root store.
func WindowsInstallScript(host string) string {
	return fmt.Sprintf(`@echo off
echo ============================================
echo  OpenPolyPrint CA Certificate Installer
echo ============================================
echo.
echo Downloading CA certificate from %s...
powershell -Command "Invoke-WebRequest -Uri 'http://%s/api/tls/ca' -OutFile '%%TEMP%%\openpolyprint-ca.pem'"
if errorlevel 1 (
    echo Failed to download CA certificate.
    pause
    exit /b 1
)
echo.
echo Installing CA certificate to Trusted Root store...
echo (This may prompt for administrator privileges)
certutil -addstore -f Root "%%TEMP%%\openpolyprint-ca.pem"
if errorlevel 1 (
    echo.
    echo Trying with current user store...
    certutil -user -addstore -f Root "%%TEMP%%\openpolyprint-ca.pem"
)
echo.
echo Done! Restart your browser for the change to take effect.
pause
`, host, host)
}

// MacInstallScript returns a shell script that downloads and installs
// the CA cert into the macOS System keychain.
func MacInstallScript(host string) string {
	return fmt.Sprintf(`#!/bin/bash
echo "============================================"
echo " OpenPolyPrint CA Certificate Installer"
echo "============================================"
echo
echo "Downloading CA certificate from %s..."
curl -s -o /tmp/openpolyprint-ca.pem http://%s/api/tls/ca
if [ $? -ne 0 ]; then
    echo "Failed to download CA certificate."
    exit 1
fi
echo
echo "Installing CA certificate to System keychain..."
echo "(This may prompt for your password)"
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /tmp/openpolyprint-ca.pem
echo
echo "Done! Restart your browser for the change to take effect."
`, host, host)
}

// LinuxInstallScript returns a shell script that downloads and installs
// the CA cert into the Linux system trust store.
func LinuxInstallScript(host string) string {
	return fmt.Sprintf(`#!/bin/bash
echo "============================================"
echo " OpenPolyPrint CA Certificate Installer"
echo "============================================"
echo
echo "Downloading CA certificate from %s..."
curl -s -o /tmp/openpolyprint-ca.pem http://%s/api/tls/ca
if [ $? -ne 0 ]; then
    echo "Failed to download CA certificate."
    exit 1
fi
echo
echo "Installing CA certificate to system trust store..."
if command -v update-ca-certificates &> /dev/null; then
    sudo cp /tmp/openpolyprint-ca.pem /usr/local/share/ca-certificates/openpolyprint-ca.crt
    sudo update-ca-certificates
elif command -v update-ca-trust &> /dev/null; then
    sudo cp /tmp/openpolyprint-ca.pem /etc/pki/ca-trust/source/anchors/openpolyprint-ca.crt
    sudo update-ca-trust
else
    echo "Unknown CA management tool. Please install the certificate manually."
    exit 1
fi
echo
echo "Done! Restart your browser for the change to take effect."
`, host, host)
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
