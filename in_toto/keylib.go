package in_toto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"golang.org/x/crypto/ed25519"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
)

/*
GenerateKeyId creates a partial key map and generates the key ID
based on the created partial key map via the SHA256 method.
The resulting keyID will be directly saved in the corresponding key object.
*/
func (k *Key) GenerateKeyId() error {
	// Create partial key map used to create the keyid
	// Unfortunately, we can't use the Key object because this also carries
	// yet unwanted fields, such as KeyId and KeyVal.Private and therefore
	// produces a different hash. We generate the keyId exactly as we do in
	// the securesystemslib  to keep interoperability between other in-toto
	// implementations.
	var keyToBeHashed = map[string]interface{}{
		"keytype":               k.KeyType,
		"scheme":                k.Scheme,
		"keyid_hash_algorithms": k.KeyIdHashAlgorithms,
		"keyval": map[string]string{
			"public": k.KeyVal.Public,
		},
	}
	keyCanonical, err := EncodeCanonical(keyToBeHashed)
	if err != nil {
		return err
	}
	// calculate sha256 and return string representation of keyId
	keyHashed := sha256.Sum256(keyCanonical)
	k.KeyId = fmt.Sprintf("%x", keyHashed)
	return nil
}

/*
ParseRSAPublicKeyFromPEM parses the passed pemBytes as e.g. read from a PEM
formatted file, and instantiates and returns the corresponding RSA public key.
If no RSA public key can be parsed, the first return value is nil and the
second return value is the error.
*/
func ParseRSAPublicKeyFromPEM(pemBytes []byte) (*rsa.PublicKey, error) {
	// TODO: There could be more key data in _, which we silently ignore here.
	// Should we handle it / fail / say something about it?
	data, _ := pem.Decode(pemBytes)
	if data == nil {
		return nil, fmt.Errorf("Could not find a public key PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(data.Bytes)
	if err != nil {
		return nil, err
	}

	//ParsePKIXPublicKey might return an rsa, dsa, or ecdsa public key
	rsaPub, isRsa := pub.(*rsa.PublicKey)
	if !isRsa {
		return nil, fmt.Errorf("We currently only support rsa keys: got '%s'",
			reflect.TypeOf(pub))
	}

	return rsaPub, nil
}

/*
ParseRSAPrivateKeyFromPEM parses the passed pemBytes as e.g. read from a PEM
formatted file, and instantiates and returns the corresponding RSA Private key.
If no RSA Private key can be parsed, the first return value is nil and the
second return value is the error.
*/
func ParseRSAPrivateKeyFromPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	// TODO: There could be more key data in _, which we silently ignore here.
	// Should we handle it / fail / say something about it?
	data, _ := pem.Decode(pemBytes)
	if data == nil {
		return nil, fmt.Errorf("Could not find a private key PEM block")
	}

	priv, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	if err != nil {
		return nil, err
	}

	return priv, nil
}

/*
LoadRSAPublicKey parses an RSA public key from a PEM formatted file at the passed
path into the Key object on which it was called.  It returns an error if the
file at path does not exist or is not a PEM formatted RSA public key.
*/
func (k *Key) LoadRSAPublicKey(path string) (err error) {
	keyFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := keyFile.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	// Read key bytes and decode PEM
	keyBytes, err := ioutil.ReadAll(keyFile)
	if err != nil {
		return err
	}

	// We only parse to see if this is indeed a pem formatted rsa public key,
	// but don't use the returned *rsa.PublicKey. Instead, we continue with
	// the original keyBytes from above.
	_, err = ParseRSAPublicKeyFromPEM(keyBytes)
	if err != nil {
		return err
	}

	// Declare values for key
	// TODO: Do not hardcode here, but define defaults elsewhere and add support
	// for parametrization
	keyType := "rsa"
	scheme := "rsassa-pss-sha256"
	keyIdHashAlgorithms := []string{"sha256", "sha512"}

	// Unmarshalling the canonicalized key into the Key object would seem natural
	// Unfortunately, our mandated canonicalization function produces a byte
	// slice that cannot be unmarshalled by Golang's json decoder, hence we have
	// to manually assign the values
	k.KeyType = keyType
	k.KeyVal = KeyVal{
		Public: strings.TrimSpace(string(keyBytes)),
	}
	k.Scheme = scheme
	k.KeyIdHashAlgorithms = keyIdHashAlgorithms
	if err := k.GenerateKeyId(); err != nil {
		return err
	}

	return nil
}

/*
LoadRSAPrivateKey parses an RSA private key from a PEM formatted file at the passed
path into the Key object on which it was called.  It returns an error if the
file at path does not exist or is not a PEM formatted RSA private key.
*/
func (k *Key) LoadRSAPrivateKey(path string) (err error) {
	keyFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := keyFile.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	// Read key bytes and decode PEM
	privateKeyBytes, err := ioutil.ReadAll(keyFile)
	if err != nil {
		return err
	}

	// We load the private key here for inferring the public key
	// from the private key. We need the public key for keyId generation
	privateKey, err := ParseRSAPrivateKeyFromPEM(privateKeyBytes)
	if err != nil {
		return err
	}
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(privateKey.Public())
	if err != nil {
		return err
	}

	// construct pemBlock
	publicKeyPemBlock := &pem.Block{
		Type:    "PUBLIC KEY",
		Headers: nil,
		Bytes:   pubKeyBytes,
	}

	publicKeyPemBlockBytes := pem.EncodeToMemory(publicKeyPemBlock)

	// Declare values for key
	// TODO: Do not hardcode here, but define defaults elsewhere and add support
	// for parametrization
	keyType := "rsa"
	scheme := "rsassa-pss-sha256"
	keyIdHashAlgorithms := []string{"sha256", "sha512"}

	// Unmarshalling the canonicalized key into the Key object would seem natural
	// Unfortunately, our mandated canonicalization function produces a byte
	// slice that cannot be unmarshalled by Golang's json decoder, hence we have
	// to manually assign the values
	k.KeyType = keyType
	k.KeyVal = KeyVal{
		Public:  strings.TrimSpace(string(publicKeyPemBlockBytes)),
		Private: strings.TrimSpace(string(privateKeyBytes)),
	}
	k.Scheme = scheme
	k.KeyIdHashAlgorithms = keyIdHashAlgorithms
	if err := k.GenerateKeyId(); err != nil {
		return err
	}

	return nil
}

/*
GenerateRSASignature generates a rsassa-pss signature, based
on the passed key and signable data. If something goes wrong
it will return an uninitialized Signature with an error.
If everything goes right, the function will return an initialized
signature with err=nil.
*/
func GenerateRSASignature(signable []byte, key Key) (Signature, error) {
	var signature Signature
	keyReader := strings.NewReader(key.KeyVal.Private)
	pemBytes, err := ioutil.ReadAll(keyReader)
	if err != nil {
		return signature, err
	}
	rsaPriv, err := ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return signature, err
	}

	hashed := sha256.Sum256(signable)

	// We use rand.Reader as secure random source for rsa.SignPSS()
	signatureBuffer, err := rsa.SignPSS(rand.Reader, rsaPriv, crypto.SHA256, hashed[:],
		&rsa.PSSOptions{SaltLength: sha256.Size, Hash: crypto.SHA256})
	if err != nil {
		return signature, err
	}

	signature.Sig = hex.EncodeToString(signatureBuffer)
	signature.KeyId = key.KeyId

	return signature, nil
}

/*
VerifyRSASignature uses the passed Key to verify the passed Signature over the
passed data.  It returns an error if the key is not a valid RSA public key or
if the signature is not valid for the data.
*/
func VerifyRSASignature(key Key, sig Signature, data []byte) error {
	// Create rsa.PublicKey object from DER encoded public key string as
	// found in the public part of the keyval part of a securesystemslib key dict
	keyReader := strings.NewReader(key.KeyVal.Public)
	pemBytes, err := ioutil.ReadAll(keyReader)
	if err != nil {
		return err
	}
	rsaPub, err := ParseRSAPublicKeyFromPEM(pemBytes)
	if err != nil {
		return err
	}

	hashed := sha256.Sum256(data)

	// Create hex bytes from the signature hex string
	sigHex, _ := hex.DecodeString(sig.Sig)

	// SecSysLib uses a SaltLength of `hashes.SHA256().digest_size`, i.e. 32
	if err := rsa.VerifyPSS(rsaPub, crypto.SHA256, hashed[:], sigHex,
		&rsa.PSSOptions{SaltLength: sha256.Size, Hash: crypto.SHA256}); err != nil {
		return err
	}

	return nil
}

/*
ParseEd25519FromPrivateJSON parses an ed25519 private key from the json string.
These ed25519 keys have the format as generated using in-toto-keygen:

	{
		"keytype: "ed25519",
		"scheme": "ed25519",
		"keyid": ...
		"keyid_hash_algorithms": [...]
		"keyval": {
			"public": "..." # 32 bytes
			"private": "..." # 32 bytes
		}
	}
*/
func ParseEd25519FromPrivateJSON(JSONString string) (Key, error) {
	var keyObj Key
	err := json.Unmarshal([]uint8(JSONString), &keyObj)
	if err != nil {
		return keyObj, fmt.Errorf("this is not a valid JSON key object")
	}

	if keyObj.KeyType != "ed25519" || keyObj.Scheme != "ed25519" {
		return keyObj, fmt.Errorf("this doesn't appear to be an ed25519 key")
	}

	// if the keyId is empty we try to generate the keyId
	if keyObj.KeyId == "" {
		if err := keyObj.GenerateKeyId(); err != nil {
			return keyObj, err
		}
	}

	if err := validatePrivateKey(keyObj); err != nil {
		return keyObj, err
	}

	// 64 hexadecimal digits => 32 bytes for the private portion of the key
	if len(keyObj.KeyVal.Private) != 64 {
		return keyObj, fmt.Errorf("the private field on this key is malformed")
	}

	return keyObj, nil
}

/*
ParseEd25519FromPublicJSON parses an ed25519 public key from the json string.
These ed25519 keys have the format as generated using in-toto-keygen:

	{
		"keytype": "ed25519",
		"scheme": "ed25519",
		"keyid_hash_algorithms": [...],
		"keyval": {"public": "..."}
	}

*/
func ParseEd25519FromPublicJSON(JSONString string) (Key, error) {
	var keyObj Key
	err := json.Unmarshal([]uint8(JSONString), &keyObj)
	if err != nil {
		return keyObj, fmt.Errorf("this is not a valid JSON key object")
	}

	if keyObj.KeyType != "ed25519" || keyObj.Scheme != "ed25519" {
		return keyObj, fmt.Errorf("this doesn't appear to be an ed25519 key")
	}

	// if the keyId is empty we try to generate the keyId
	if keyObj.KeyId == "" {
		if err := keyObj.GenerateKeyId(); err != nil {
			return keyObj, err
		}
	}

	if err := validatePubKey(keyObj); err != nil {
		return keyObj, err
	}

	// 64 hexadecimal digits => 32 bytes for the public portion of the key
	if len(keyObj.KeyVal.Public) != 64 {
		return keyObj, fmt.Errorf("the public field on this key is malformed")
	}

	return keyObj, nil
}

/*
GenerateEd25519Signature creates an ed25519 signature using the key and the
signable buffer provided. It returns an error if the underlying signing library
fails.
*/
func GenerateEd25519Signature(signable []byte, key Key) (Signature, error) {

	var signature Signature

	seed, err := hex.DecodeString(key.KeyVal.Private)
	if err != nil {
		return signature, err
	}
	privkey := ed25519.NewKeyFromSeed(seed)
	signatureBuffer := ed25519.Sign(privkey, signable)

	signature.Sig = hex.EncodeToString(signatureBuffer)
	signature.KeyId = key.KeyId

	return signature, nil
}

/*
VerifyEd25519Signature uses the passed Key to verify the passed Signature over the
passed data. It returns an error if the key is not a valid ed25519 public key or
if the signature is not valid for the data.
*/
func VerifyEd25519Signature(key Key, sig Signature, data []byte) error {
	pubHex, err := hex.DecodeString(key.KeyVal.Public)
	if err != nil {
		return err
	}
	sigHex, err := hex.DecodeString(sig.Sig)
	if err != nil {
		return err
	}
	if ok := ed25519.Verify(pubHex, data, sigHex); !ok {
		return errors.New("invalid ed25519 signature")
	}
	return nil
}

/* LoadEd25519PublicKey loads an ed25519 pub key file
and parses it via ParseEd25519FromPublicJSON.
The pub key file has to be in the in-toto PublicJSON format
For example:

	{
		"keytype": "ed25519",
		"scheme": "ed25519",
		"keyid_hash_algorithms": ["sha256", "sha512"],
		"keyval":
		{
			"public": "8c93f633f2378cc64dd7cbb0ed35eac59e1f28065f90cbbddb59878436fec037"
		}
	}

*/
func (k *Key) LoadEd25519PublicKey(path string) (err error) {
	keyFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := keyFile.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	keyBytes, err := ioutil.ReadAll(keyFile)
	if err != nil {
		return err
	}
	// contrary to LoadRSAPublicKey we use the returned key object
	keyObj, err := ParseEd25519FromPublicJSON(string(keyBytes))
	if err != nil {
		return err
	}
	// I am not sure if there is a faster way to fill the Key struct
	// without touching the ParseEd25519FromPrivateJSON function
	k.KeyId = keyObj.KeyId
	k.KeyType = keyObj.KeyType
	k.KeyIdHashAlgorithms = keyObj.KeyIdHashAlgorithms
	k.KeyVal = keyObj.KeyVal
	k.Scheme = keyObj.Scheme
	return nil
}

/* LoadEd25519PrivateKey loads an ed25519 private key file
and parses it via ParseEd25519FromPrivateJSON.
ParseEd25519FromPrivateKey does not support encrypted private keys yet.
The private key file has to be in the in-toto PrivateJSON format
For example:

	{
		"keytype": "ed25519",
		"scheme": "ed25519",
		"keyid": "d7c0baabc90b7bf218aa67461ec0c3c7f13a8a5d8552859c8fafe41588be01cf",
		"keyid_hash_algorithms": ["sha256", "sha512"],
		"keyval":
		{
			"public": "8c93f633f2378cc64dd7cbb0ed35eac59e1f28065f90cbbddb59878436fec037",
			"private": "4cedf4d3369f8c83af472d0d329aedaa86265b74efb74b708f6a1ed23f290162"
		}
	}

*/
func (k *Key) LoadEd25519PrivateKey(path string) (err error) {
	// TODO: Support for encrypted private Keys
	// See also: https://github.com/secure-systems-lab/securesystemslib/blob/01a0c95af5f458235f96367922357958bfcf7b01/securesystemslib/keys.py#L1309
	keyFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := keyFile.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	keyBytes, err := ioutil.ReadAll(keyFile)
	if err != nil {
		return err
	}
	// contrary to LoadRSAPublicKey we use the returned key object
	keyObj, err := ParseEd25519FromPrivateJSON(string(keyBytes))
	if err != nil {
		return err
	}
	// I am not sure if there is a faster way to fill the Key struct
	// without touching the ParseEd25519FromPrivateJSON function
	k.KeyId = keyObj.KeyId
	k.KeyType = keyObj.KeyType
	k.KeyIdHashAlgorithms = keyObj.KeyIdHashAlgorithms
	k.KeyVal = keyObj.KeyVal
	k.Scheme = keyObj.Scheme
	return nil
}
