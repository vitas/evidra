package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

var (
	ErrNoSigningKey    = errors.New("evidence.NewSigner: signing key required in production (set EVIDRA_SIGNING_KEY or EVIDRA_SIGNING_KEY_PATH)")
	ErrInvalidKey      = errors.New("evidence.NewSigner: invalid Ed25519 private key")
	ErrInvalidPEMBlock = errors.New("evidence.NewSigner: PEM block is not a PRIVATE KEY")
)

// Signer signs and verifies evidence payloads with Ed25519.
type Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// SignerConfig holds the parameters for creating a Signer.
type SignerConfig struct {
	// Base64-encoded Ed25519 private key (from EVIDRA_SIGNING_KEY).
	KeyBase64 string

	// Path to a PEM-encoded Ed25519 private key file (from EVIDRA_SIGNING_KEY_PATH).
	KeyPath string

	// When true, generate an ephemeral in-memory key if no key is provided.
	// Only allowed when EVIDRA_ENVIRONMENT=development.
	DevMode bool
}

// NewSigner creates a Signer from the provided configuration.
//
// Key resolution order:
//  1. KeyBase64 -- raw base64-encoded Ed25519 seed or private key
//  2. KeyPath -- PEM file containing an Ed25519 private key
//  3. DevMode=true -- generate ephemeral key (logs warning)
//  4. None of the above -- return ErrNoSigningKey
func NewSigner(cfg SignerConfig) (*Signer, error) {
	switch {
	case cfg.KeyBase64 != "":
		return signerFromBase64(cfg.KeyBase64)
	case cfg.KeyPath != "":
		return signerFromPEM(cfg.KeyPath)
	case cfg.DevMode:
		return signerEphemeral()
	default:
		return nil, ErrNoSigningKey
	}
}

// Sign signs the payload and returns the raw Ed25519 signature.
func (s *Signer) Sign(payload []byte) []byte {
	return ed25519.Sign(s.privateKey, payload)
}

// Verify reports whether sig is a valid signature of payload by this signer's public key.
func (s *Signer) Verify(payload, sig []byte) bool {
	return ed25519.Verify(s.publicKey, payload, sig)
}

// PublicKey returns the Ed25519 public key.
func (s *Signer) PublicKey() ed25519.PublicKey {
	return s.publicKey
}

// PublicKeyPEM returns the public key encoded as a PEM block.
func (s *Signer) PublicKeyPEM() ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(s.publicKey)
	if err != nil {
		return nil, fmt.Errorf("evidence.PublicKeyPEM: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}), nil
}

// LoadPublicKeyPEM reads a PEM-encoded Ed25519 public key from a file.
func LoadPublicKeyPEM(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evidence.LoadPublicKeyPEM: read file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("evidence.LoadPublicKeyPEM: PEM block is not a PUBLIC KEY")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("evidence.LoadPublicKeyPEM: parse PKIX: %w", err)
	}

	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("evidence.LoadPublicKeyPEM: key is not Ed25519")
	}

	return pub, nil
}

func signerFromBase64(encoded string) (*Signer, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("evidence.NewSigner: base64 decode: %w", err)
	}

	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize:
		priv = ed25519.PrivateKey(raw)
	default:
		return nil, ErrInvalidKey
	}

	return &Signer{
		privateKey: priv,
		publicKey:  priv.Public().(ed25519.PublicKey),
	}, nil
}

func signerFromPEM(path string) (*Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evidence.NewSigner: read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, ErrInvalidPEMBlock
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("evidence.NewSigner: parse PKCS8: %w", err)
	}

	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrInvalidKey
	}

	return &Signer{
		privateKey: priv,
		publicKey:  priv.Public().(ed25519.PublicKey),
	}, nil
}

func signerEphemeral() (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("evidence.NewSigner: generate ephemeral key: %w", err)
	}

	slog.Warn("using ephemeral signing key -- evidence will not survive restart")

	return &Signer{
		privateKey: priv,
		publicKey:  pub,
	}, nil
}
