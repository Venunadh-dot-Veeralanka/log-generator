package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode"
)

const underscore string = "_"
const (
	DecryptionKey = "SnappyFlow123456"
)

func unpad(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func aesCBCDecrypt(encryptData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()

	if len(encryptData) < blockSize {
		return nil, errors.New("ciphertext too short")
	}
	iv := encryptData[:blockSize]
	encryptData = encryptData[blockSize:]

	if len(encryptData)%blockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	mode.CryptBlocks(encryptData, encryptData)
	encryptData = unpad(encryptData)
	return encryptData, nil
}

func Decrypt(rawData string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(rawData)
	if err != nil {
		return "", err
	}
	dnData, err := aesCBCDecrypt(data, key)
	if err != nil {
		return "", err
	}
	return string(dnData), nil
}

func createTargetsFromKey(c *Config) (SnappyFlowKeyData, error) {
	data, err := Decrypt(c.ESKey, []byte(DecryptionKey))
	if err != nil {
		fmt.Println(err)
		return SnappyFlowKeyData{}, err
	}

	var keyData SnappyFlowKeyData
	err = json.Unmarshal([]byte(data), &keyData)
	if err != nil {
		fmt.Printf("unable to unmarshal key data, %s\n", err)
		return SnappyFlowKeyData{}, err
	}

	return keyData, nil
}

// GetAPMName return names formatted for apm
func GetAPMName(project string) string {
	rp := []rune(project)
	rpn := []rune{}
	for i := 0; i < len(rp); i++ {
		switch letter := rp[i]; {
		case unicode.IsLetter(letter) && unicode.IsUpper(letter):
			rpn = append(rpn, []rune(underscore)...)
			rpn = append(rpn, unicode.ToLower(letter))
		case string(letter) == "_":
			rpn = append(rpn, []rune(underscore)...)
			rpn = append(rpn, []rune(underscore)...)
		default:
			rpn = append(rpn, letter)
		}
	}
	return string(rpn)
}
