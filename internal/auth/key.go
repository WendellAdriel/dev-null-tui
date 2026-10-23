package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/pem"
	"errors"

	"golang.org/x/crypto/ssh"
)

type Key struct {
	signer  ssh.Signer
	private ed25519.PrivateKey
}

func GenerateKey() (*Key, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(private)
	return &Key{signer: signer, private: private}, err
}

func parseKey(data []byte) (*Key, error) {
	private, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		return nil, errors.New("cannot read device key; disconnect locally before reconnecting")
	}
	ed, ok := private.(*ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("device key must be Ed25519")
	}
	signer, err := ssh.NewSignerFromKey(*ed)
	return &Key{signer: signer, private: *ed}, err
}

func (k *Key) encode() ([]byte, error) {
	return encodeKey(k.private)
}

func (k *Key) Fingerprint() string { return ssh.FingerprintSHA256(k.signer.PublicKey()) }

func (k *Key) Sign(message []byte) (string, error) {
	digest := sha512.Sum512(message)
	payload := append([]byte("SSHSIG"), ssh.Marshal(struct {
		Namespace, Reserved, Hash string
		Digest                    []byte
	}{"devnull-api", "", "sha512", digest[:]})...)
	sig, err := k.signer.Sign(rand.Reader, payload)
	if err != nil {
		return "", err
	}
	blob := append([]byte("SSHSIG"), ssh.Marshal(struct {
		Version                   uint32
		PublicKey                 []byte
		Namespace, Reserved, Hash string
		Signature                 []byte
	}{1, k.signer.PublicKey().Marshal(), "devnull-api", "", "sha512", ssh.Marshal(sig)})...)
	return base64.StdEncoding.EncodeToString(blob), nil
}

func encodeKey(private ed25519.PrivateKey) ([]byte, error) {
	block, err := ssh.MarshalPrivateKey(private, "dev-null-cli device")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}
