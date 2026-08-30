package fixture

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/rc4"
)

func legacyBlock(key []byte) (cipher.Block, error) {
	// ruleid: go-weak-cipher
	return des.NewCipher(key)
}

func tripleBlock(key []byte) (cipher.Block, error) {
	// ruleid: go-weak-cipher
	return des.NewTripleDESCipher(key)
}

func stream(key []byte) (*rc4.Cipher, error) {
	// ruleid: go-weak-cipher
	return rc4.NewCipher(key)
}

func modernBlock(key []byte) (cipher.Block, error) {
	// ok: go-weak-cipher
	return aes.NewCipher(key)
}
