package hashidlib

import (
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	id := int32(12345)
	h := Encode(id)
	if h == "" {
		t.Error("expected non-empty hash")
	}

	decoded, err := Decode(h)
	if err != nil {
		t.Errorf("Decode failed: %v", err)
	}
	if decoded != id {
		t.Errorf("expected %d, got %d", id, decoded)
	}
}

func TestEncodeZero(t *testing.T) {
	if Encode(0) != "" {
		t.Error("expected empty string for ID 0")
	}
}

func TestDecodeEmpty(t *testing.T) {
	decoded, err := Decode("")
	if err != nil {
		t.Errorf("expected no error for empty string, got %v", err)
	}
	if decoded != 0 {
		t.Errorf("expected 0 for empty string, got %d", decoded)
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode("invalid-hash")
	if err == nil {
		t.Error("expected error for invalid hash")
	}
}
