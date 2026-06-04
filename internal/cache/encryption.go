package cache

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	granolaEncryptedSuffix        = ".enc"
	safeStoragePrefix             = "v10"
	safeStoragePBKDF2Iterations   = 1003
	safeStorageDerivedKeyLength   = 16
	granolaCacheEncryptionKeySize = 32
	granolaCacheGCMNonceSize      = 12
	granolaCacheGCMTagSize        = 16
)

var (
	loadDEKForCachePathFunc       = loadDEKForCachePath
	lookupSafeStoragePasswordFunc = lookupSafeStoragePassword
	writeFileFunc                 = os.WriteFile
	mkdirAllFunc                  = os.MkdirAll
)

type keychainQuery struct {
	args []string
}

func DumpDecryptedCache(cachePath string, outputPath string) error {
	_, encPath := relatedCachePaths(cachePath)
	if !cacheFileExists(encPath) {
		return fmt.Errorf("encrypted cache file not found: %s", encPath)
	}

	data, err := readEncryptedCacheData(encPath)
	if err != nil {
		return err
	}

	if err := mkdirAllFunc(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}
	if err := writeFileFunc(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("cannot write decrypted cache file: %w", err)
	}
	return nil
}

func readCacheData(cachePath string) ([]byte, error) {
	plainPath, encPath := relatedCachePaths(cachePath)
	if shouldPreferEncryptedCache(cachePath, plainPath, encPath) {
		data, err := readEncryptedCacheData(encPath)
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	data, err := os.ReadFile(plainPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read cache file: %w", err)
	}
	return data, nil
}

func relatedCachePaths(cachePath string) (plainPath string, encPath string) {
	trimmed := strings.TrimSpace(cachePath)
	if strings.HasSuffix(trimmed, granolaEncryptedSuffix) {
		return strings.TrimSuffix(trimmed, granolaEncryptedSuffix), trimmed
	}
	return trimmed, trimmed + granolaEncryptedSuffix
}

func shouldPreferEncryptedCache(cachePath string, plainPath string, encPath string) bool {
	if strings.HasSuffix(strings.TrimSpace(cachePath), granolaEncryptedSuffix) {
		return true
	}
	return cacheFileExists(encPath)
}

func readEncryptedCacheData(encPath string) ([]byte, error) {
	payload, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read encrypted cache file: %w", err)
	}

	dek, err := loadDEKForCachePathFunc(encPath)
	if err != nil {
		return nil, fmt.Errorf("cannot unwrap data encryption key: %w", err)
	}

	plaintext, err := decryptGranolaEncryptedPayload(payload, dek)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt encrypted cache payload: %w", err)
	}
	return plaintext, nil
}

func loadDEKForCachePath(cachePath string) ([]byte, error) {
	dekPath := filepath.Join(filepath.Dir(cachePath), "storage.dek")
	payload, err := os.ReadFile(dekPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", dekPath, err)
	}

	plaintext, err := decryptSafeStorageBlob(payload)
	if err != nil {
		return nil, err
	}

	dek, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(plaintext)))
	if err != nil {
		return nil, fmt.Errorf("invalid base64 DEK payload: %w", err)
	}
	if len(dek) != granolaCacheEncryptionKeySize {
		return nil, fmt.Errorf("unexpected DEK length: got %d bytes want %d", len(dek), granolaCacheEncryptionKeySize)
	}
	return dek, nil
}

func decryptSafeStorageBlob(payload []byte) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("Granola encrypted cache decryption is currently supported only on macOS")
	}

	password, err := lookupSafeStoragePasswordFunc()
	if err != nil {
		return nil, err
	}
	return decryptSafeStorageBlobWithPassword(payload, password)
}

func decryptSafeStorageBlobWithPassword(payload []byte, password string) ([]byte, error) {
	if len(payload) <= len(safeStoragePrefix) || string(payload[:len(safeStoragePrefix)]) != safeStoragePrefix {
		return nil, fmt.Errorf("unsupported safeStorage prefix")
	}

	key := pbkdf2SHA1([]byte(password), []byte("saltysalt"), safeStoragePBKDF2Iterations, safeStorageDerivedKeyLength)
	iv := bytes.Repeat([]byte(" "), aes.BlockSize)
	return decryptAESCBCPKCS7(payload[len(safeStoragePrefix):], key, iv)
}

func lookupSafeStoragePassword() (string, error) {
	if override := strings.TrimSpace(os.Getenv("GRANOLA_SAFE_STORAGE_PASSWORD")); override != "" {
		return override, nil
	}

	var attempts []string
	for _, query := range buildKeychainQueries() {
		out, err := exec.Command("security", query.args...).Output()
		if err == nil {
			password := strings.TrimSpace(string(out))
			if password != "" {
				return password, nil
			}
		}
		if err != nil {
			attempts = append(attempts, strings.Join(query.args, " "))
		}
	}

	if len(attempts) == 0 {
		return "", errors.New("safe storage password not found in macOS Keychain")
	}
	return "", fmt.Errorf("safe storage password not found in macOS Keychain; set GRANOLA_SAFE_STORAGE_PASSWORD to override")
}

func buildKeychainQueries() []keychainQuery {
	queries := make([]keychainQuery, 0, 16)
	seen := make(map[string]struct{})
	add := func(parts ...string) {
		key := strings.Join(parts, "\x00")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, keychainQuery{args: parts})
	}

	envLabel := strings.TrimSpace(os.Getenv("GRANOLA_SAFE_STORAGE_LABEL"))
	envService := strings.TrimSpace(os.Getenv("GRANOLA_SAFE_STORAGE_SERVICE"))
	envAccount := strings.TrimSpace(os.Getenv("GRANOLA_SAFE_STORAGE_ACCOUNT"))

	if envLabel != "" {
		add("find-generic-password", "-l", envLabel, "-w")
	}
	if envService != "" {
		if envAccount != "" {
			add("find-generic-password", "-a", envAccount, "-s", envService, "-w")
		}
		add("find-generic-password", "-s", envService, "-w")
	}

	names := []string{
		"Granola Safe Storage",
		"com.granola.app Safe Storage",
		"Electron Safe Storage",
	}
	accounts := []string{
		"Granola",
		"com.granola.app",
		"Electron",
	}

	for _, name := range names {
		add("find-generic-password", "-l", name, "-w")
		add("find-generic-password", "-s", name, "-w")
		for _, account := range accounts {
			add("find-generic-password", "-a", account, "-s", name, "-w")
		}
	}

	return queries
}

func decryptGranolaEncryptedPayload(payload []byte, dek []byte) ([]byte, error) {
	if len(dek) != granolaCacheEncryptionKeySize {
		return nil, fmt.Errorf("unexpected DEK length: got %d bytes want %d", len(dek), granolaCacheEncryptionKeySize)
	}
	if len(payload) < granolaCacheGCMNonceSize+granolaCacheGCMTagSize {
		return nil, errors.New("encrypted payload is too short")
	}

	nonce := payload[:granolaCacheGCMNonceSize]
	ciphertext := payload[granolaCacheGCMNonceSize : len(payload)-granolaCacheGCMTagSize]
	tag := payload[len(payload)-granolaCacheGCMTagSize:]

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithTagSize(block, granolaCacheGCMTagSize)
	if err != nil {
		return nil, err
	}

	sealed := make([]byte, 0, len(ciphertext)+len(tag))
	sealed = append(sealed, ciphertext...)
	sealed = append(sealed, tag...)
	return gcm.Open(nil, nonce, sealed, nil)
}

func decryptAESCBCPKCS7(ciphertext []byte, key []byte, iv []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext length must be a non-zero multiple of the AES block size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext, aes.BlockSize)
}

func pkcs7Unpad(plaintext []byte, blockSize int) ([]byte, error) {
	if len(plaintext) == 0 || len(plaintext)%blockSize != 0 {
		return nil, errors.New("invalid PKCS#7 plaintext length")
	}

	padLen := int(plaintext[len(plaintext)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(plaintext) {
		return nil, errors.New("invalid PKCS#7 padding")
	}
	for _, b := range plaintext[len(plaintext)-padLen:] {
		if int(b) != padLen {
			return nil, errors.New("invalid PKCS#7 padding")
		}
	}
	return plaintext[:len(plaintext)-padLen], nil
}

func pbkdf2SHA1(password []byte, salt []byte, iterations int, keyLen int) []byte {
	hashLen := sha1.Size
	blockCount := (keyLen + hashLen - 1) / hashLen
	derived := make([]byte, 0, blockCount*hashLen)

	for blockNum := 1; blockNum <= blockCount; blockNum++ {
		u := pbkdf2Block(password, salt, iterations, blockNum)
		derived = append(derived, u...)
	}

	return derived[:keyLen]
}

func pbkdf2Block(password []byte, salt []byte, iterations int, blockNum int) []byte {
	mac := hmac.New(sha1.New, password)
	mac.Write(salt)
	mac.Write([]byte{
		byte(blockNum >> 24),
		byte(blockNum >> 16),
		byte(blockNum >> 8),
		byte(blockNum),
	})
	u := mac.Sum(nil)
	block := append([]byte(nil), u...)

	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha1.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range block {
			block[j] ^= u[j]
		}
	}

	return block
}

func fileModTime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func cacheFileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
