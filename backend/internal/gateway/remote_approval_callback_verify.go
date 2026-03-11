package gateway

// remote_approval_callback_verify.go — Phase 8 回调验签
//
// 纯函数，不修改任何现有代码。提供：
//   - 钉钉互动卡片回调 HMAC-SHA256 验签
//   - 企业微信回调 SHA1 签名验证 + AES-256-CBC 解密
//
// 参考文档：
//   - 钉钉: apiSecret + timestamp → HMAC-SHA256 → Base64 → 对比 x-ddpaas-signature
//   - 企微: SHA1(sort(token, timestamp, nonce, encrypt)) → 对比 msg_signature
//         AES-256-CBC(EncodingAESKey) → 解密 encrypt 获取明文

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// ---------- 钉钉 HMAC-SHA256 验签 ----------

// VerifyDingTalkSignature 验证钉钉互动卡片回调签名。
//
// 算法：HMAC-SHA256(apiSecret, timestamp+"\n"+apiSecret) → Base64 → 对比 signature
// Header: x-ddpaas-signature-timestamp / x-ddpaas-signature
func VerifyDingTalkSignature(apiSecret, timestamp, signature string) bool {
	if apiSecret == "" || timestamp == "" || signature == "" {
		return false
	}

	stringToSign := timestamp + "\n" + apiSecret
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(stringToSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// ---------- 企业微信签名验证 ----------

// VerifyWeComSignature 验证企业微信回调签名。
//
// 算法：sort(token, timestamp, nonce, encrypt) → SHA1 拼接 → 对比 msg_signature
func VerifyWeComSignature(token, timestamp, nonce, msgSignature, encrypt string) bool {
	if token == "" || timestamp == "" || nonce == "" || msgSignature == "" {
		return false
	}

	parts := []string{token, timestamp, nonce, encrypt}
	sort.Strings(parts)
	joined := strings.Join(parts, "")

	hash := sha1.New()
	hash.Write([]byte(joined))
	expected := fmt.Sprintf("%x", hash.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(msgSignature))
}

// ---------- 企业微信 AES 解密 ----------

// DecryptWeComMessage 使用 EncodingAESKey 解密企业微信回调密文。
//
// EncodingAESKey 是 43 位 Base64 编码，补 "=" 后解码得到 32 字节 AES 密钥。
// 使用 AES-256-CBC，IV = AESKey[:16]，PKCS#7 去填充。
func DecryptWeComMessage(encodingAESKey, ciphertext string) ([]byte, error) {
	if encodingAESKey == "" || ciphertext == "" {
		return nil, fmt.Errorf("encodingAESKey and ciphertext must not be empty")
	}

	// 补 "=" 并 Base64 解码得到 AES Key
	aesKey, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("invalid EncodingAESKey: %w", err)
	}
	if len(aesKey) != 32 {
		return nil, fmt.Errorf("AESKey length must be 32, got %d", len(aesKey))
	}

	// Base64 解码密文
	cipherData, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext base64: %w", err)
	}

	// AES-256-CBC 解密
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}

	if len(cipherData) < aes.BlockSize || len(cipherData)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext size")
	}

	iv := aesKey[:16] // IV = AESKey 前 16 字节
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(cipherData))
	mode.CryptBlocks(plaintext, cipherData)

	// PKCS#7 去填充
	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("pkcs7 unpad failed: %w", err)
	}

	// 企业微信格式：前 16 字节随机，4 字节 content 长度（网络字节序），content，corp_id
	if len(plaintext) < 20 {
		return nil, fmt.Errorf("decrypted data too short")
	}

	contentLen := int(plaintext[16])<<24 | int(plaintext[17])<<16 | int(plaintext[18])<<8 | int(plaintext[19])
	if 20+contentLen > len(plaintext) {
		return nil, fmt.Errorf("content length mismatch")
	}

	return plaintext[20 : 20+contentLen], nil
}

// pkcs7Unpad 移除 PKCS#7 填充。
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > len(data) || padLen > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding length: %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("invalid padding byte")
		}
	}
	return data[:len(data)-padLen], nil
}
