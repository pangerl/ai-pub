package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// insecureKeyWarned 确保弱密钥回退告警只打印一次（worker 与 router 各自构造 Box）。
var insecureKeyWarned sync.Once

type Box struct {
	key [32]byte
}

func NewBox(secret string) Box {
	if secret == "" {
		insecureKeyWarned.Do(func() {
			slog.Warn("APP_ENCRYPTION_KEY 未设置，回退到不安全的开发默认密钥；凭据可被任何持有二进制者解密，生产环境必须显式提供")
		})
		secret = "dev-secret-change-me"
	}
	return Box{key: sha256.Sum256([]byte(secret))}
}

func (b Box) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(b.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func (b Box) Decrypt(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(b.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted value is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func Redact(value string) string {
	if value == "" {
		return value
	}
	return "[redacted]"
}
