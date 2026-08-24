package mqtt

import (
	"crypto/x509"
	"fmt"

	"github.com/lucas/openpolyprint/internal/anker/proto/crypto"

	"github.com/google/uuid"
)

func newUUID() string {
	return uuid.New().String()
}

func newCertPool(caCert []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}
	return pool, nil
}

func mqttChecksumRemove(payload []byte) ([]byte, error) {
	return crypto.MQTTChecksumRemove(payload)
}

func mqttChecksumAdd(payload []byte) []byte {
	return crypto.MQTTChecksumAdd(payload)
}

func mqttAESEncrypt(data, key []byte) ([]byte, error) {
	return crypto.MQTTAESEncrypt(data, key)
}

func mqttAESDecrypt(data, key []byte) ([]byte, error) {
	return crypto.MQTTAESDecrypt(data, key)
}

func mqttAESDecryptWithIV(data, key, iv []byte) ([]byte, error) {
	return crypto.MQTTAESDecryptWithIV(data, key, iv)
}

func aesDecryptBlock(block, key []byte) ([]byte, error) {
	return crypto.AESDecryptBlock(block, key)
}
