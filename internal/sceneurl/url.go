// Package sceneurl carries an existing impact-scene JSON document in a URL.
package sceneurl

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	MaxLength       = 10_000
	MaxDecodedBytes = 16 << 20
	WebPrefix       = "https://tldiagram.com/app/compare#v1="
	LocalPrefix     = "tld://compare#v1="
)

// Encode preserves the JSON document exactly, including optional audit fields.
func Encode(document []byte, local bool) (string, error) {
	prefix := WebPrefix
	if local {
		prefix = LocalPrefix
	}
	return encode(document, prefix)
}

// EncodeAt targets the viewer served below an explicit HTTP(S) application
// base URL. The full destination is included in the transport size budget.
func EncodeAt(document []byte, baseURL string) (string, error) {
	prefix, err := WebURLPrefix(baseURL)
	if err != nil {
		return "", err
	}
	return encode(document, prefix)
}

func WebURLPrefix(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(baseURL, "#") {
		return "", fmt.Errorf("comparison URL base must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return u.JoinPath("compare").String() + "#v1=", nil
}

func encode(document []byte, prefix string) (string, error) {
	if len(document) > MaxDecodedBytes {
		return "", fmt.Errorf("scene is %d bytes; URL scenes support at most %d decoded bytes", len(document), MaxDecodedBytes)
	}
	var compressed bytes.Buffer
	w, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(document); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	result := prefix + base64.RawURLEncoding.EncodeToString(compressed.Bytes())
	if err := CheckLength(result); err != nil {
		return "", err
	}
	return result, nil
}

func CheckLength(value string) error {
	if len(value) > MaxLength {
		return fmt.Errorf("comparison URL is %d characters; limit is %d", len(value), MaxLength)
	}
	return nil
}

// LocalRoute accepts only our local comparison URL, never an arbitrary route.
// Decompression and scene validation happen in the shared frontend viewer.
func LocalRoute(value string) (string, error) {
	if err := CheckLength(value); err != nil {
		return "", err
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "tld" || u.Host != "compare" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Opaque != "" {
		return "", fmt.Errorf("invalid TLD comparison URL")
	}
	if !strings.HasPrefix(value, LocalPrefix) || u.RawFragment != "" {
		return "", fmt.Errorf("unsupported comparison URL encoding; expected v1")
	}
	token := strings.TrimPrefix(value, LocalPrefix)
	if token == "" {
		return "", fmt.Errorf("comparison URL has no scene")
	}
	if _, err := base64.RawURLEncoding.Strict().DecodeString(token); err != nil {
		return "", fmt.Errorf("invalid comparison URL payload")
	}
	return "/compare#v1=" + token, nil
}
