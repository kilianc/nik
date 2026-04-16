package secrets

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	keySize   = 32
	nonceSize = 24
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Store struct {
	keyPath  string
	dataPath string
}

func New(home string) *Store {
	return &Store{
		keyPath:  filepath.Join(home, ".secrets.key"),
		dataPath: filepath.Join(home, "secrets.enc"),
	}
}

func (s *Store) Get(name string) (string, error) {
	m, err := s.readAll()
	if err != nil {
		return "", err
	}

	val, ok := m[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}

	return val, nil
}

func (s *Store) Set(name, value string) error {
	err := validateName(name)
	if err != nil {
		return err
	}

	m, err := s.readAll()
	if err != nil {
		return err
	}

	m[name] = value
	return s.writeAll(m)
}

func (s *Store) Delete(name string) error {
	m, err := s.readAll()
	if err != nil {
		return err
	}

	if _, ok := m[name]; !ok {
		return fmt.Errorf("secret %q not found", name)
	}

	delete(m, name)
	return s.writeAll(m)
}

func (s *Store) List() ([]string, error) {
	m, err := s.readAll()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)

	return names, nil
}

func (s *Store) readAll() (map[string]string, error) {
	ciphertext, err := os.ReadFile(s.dataPath)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secrets file: %w", err)
	}

	key, err := s.loadKey()
	if err != nil {
		return nil, err
	}

	plaintext, err := decrypt(ciphertext, key)
	if err != nil {
		return nil, err
	}

	var m map[string]string
	err = json.Unmarshal(plaintext, &m)
	if err != nil {
		return nil, fmt.Errorf("parse secrets: %w", err)
	}

	return m, nil
}

func (s *Store) writeAll(m map[string]string) error {
	plaintext, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	key, err := s.loadOrCreateKey()
	if err != nil {
		return err
	}

	ciphertext, err := encrypt(plaintext, key)
	if err != nil {
		return err
	}

	tmp := s.dataPath + ".tmp"
	err = os.WriteFile(tmp, ciphertext, 0o600)
	if err != nil {
		return fmt.Errorf("write secrets tmp: %w", err)
	}

	err = os.Rename(tmp, s.dataPath)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename secrets file: %w", err)
	}

	return nil
}

func (s *Store) loadKey() ([keySize]byte, error) {
	data, err := os.ReadFile(s.keyPath)
	if err != nil {
		return [keySize]byte{}, err
	}

	if len(data) != keySize {
		return [keySize]byte{}, fmt.Errorf("invalid key file: expected %d bytes, got %d", keySize, len(data))
	}

	var key [keySize]byte
	copy(key[:], data)
	return key, nil
}

func (s *Store) loadOrCreateKey() ([keySize]byte, error) {
	key, err := s.loadKey()
	if err == nil {
		return key, nil
	}

	if !os.IsNotExist(err) {
		return [keySize]byte{}, fmt.Errorf("load key: %w", err)
	}

	_, err = rand.Read(key[:])
	if err != nil {
		return [keySize]byte{}, fmt.Errorf("generate key: %w", err)
	}

	dir := filepath.Dir(s.keyPath)
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return [keySize]byte{}, fmt.Errorf("create key dir: %w", err)
	}

	err = os.WriteFile(s.keyPath, key[:], 0o600)
	if err != nil {
		return [keySize]byte{}, fmt.Errorf("write key file: %w", err)
	}

	return key, nil
}

func encrypt(plaintext []byte, key [keySize]byte) ([]byte, error) {
	var nonce [nonceSize]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	sealed := secretbox.Seal(nonce[:], plaintext, &nonce, &key)
	return sealed, nil
}

func decrypt(data []byte, key [keySize]byte) ([]byte, error) {
	if len(data) < nonceSize+secretbox.Overhead {
		return nil, fmt.Errorf("ciphertext too short")
	}

	var nonce [nonceSize]byte
	copy(nonce[:], data[:nonceSize])

	plaintext, ok := secretbox.Open(nil, data[nonceSize:], &nonce, &key)
	if !ok {
		return nil, fmt.Errorf("decrypt failed: invalid key or corrupted data")
	}

	return plaintext, nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("empty secret name")
	}

	if !validName.MatchString(name) {
		return fmt.Errorf("invalid secret name %q: alphanumeric, underscores, and hyphens only", name)
	}

	return nil
}
