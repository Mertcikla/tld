package sceneurl

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSceneURLRoundTrip(t *testing.T) {
	document, err := os.ReadFile("../mermaid/fixtures/scene-url.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, local := range []bool{false, true} {
		link, err := Encode(document, local)
		if err != nil {
			t.Fatal(err)
		}
		prefix := WebPrefix
		if local {
			prefix = LocalPrefix
		}
		compressed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, prefix))
		if err != nil {
			t.Fatal(err)
		}
		r, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(document, decoded) {
			t.Fatal("JSON bytes changed")
		}
		again, err := Encode(document, local)
		if err != nil || again != link {
			t.Fatal("encoding not deterministic", err)
		}
		if local {
			route, err := LocalRoute(link)
			if err != nil || route != "/compare#v1="+strings.TrimPrefix(link, prefix) {
				t.Fatalf("route %q: %v", route, err)
			}
		}
	}
}

func TestURLLengthBoundary(t *testing.T) {
	if err := CheckLength(strings.Repeat("a", MaxLength)); err != nil {
		t.Fatal(err)
	}
	if err := CheckLength(strings.Repeat("a", MaxLength+1)); err == nil || !strings.Contains(err.Error(), "10001") || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("error=%v", err)
	}
	if _, err := Encode(bytes.Repeat([]byte("a"), MaxDecodedBytes+1), false); err == nil {
		t.Fatal("decoded size not bounded")
	}
}

func TestLocalRouteRejectsOtherDestinations(t *testing.T) {
	for _, value := range []string{"https://tldiagram.com/app/compare#v1=YQ", "tld://other#v1=YQ", "tld://compare/path#v1=YQ", "tld://compare:80#v1=YQ", "tld://user@compare#v1=YQ", "tld://compare?x=y#v1=YQ", "tld://compare#v2=YQ", "tld://compare#v1=", "tld://compare#v1=YR", "tld://compare#v1=YQ%3D%3D", LocalPrefix + strings.Repeat("a", MaxLength)} {
		if _, err := LocalRoute(value); err == nil {
			t.Errorf("accepted %q", value[:min(len(value), 80)])
		}
	}
}

func TestEncodeAtUsesApplicationBaseAndFullURLBudget(t *testing.T) {
	document := []byte(`{"comparison":{}}`)
	standard, err := Encode(document, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ base, prefix string }{
		{"http://127.0.0.1:8060", "http://127.0.0.1:8060/compare#v1="},
		{"http://localhost:5173/app/", "http://localhost:5173/app/compare#v1="},
		{"https://tldiagram.com/app", WebPrefix},
	} {
		got, err := EncodeAt(document, test.base)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.prefix+strings.TrimPrefix(standard, WebPrefix) {
			t.Fatalf("unexpected destination for %s", test.base)
		}
	}
	if _, err := EncodeAt(document, "https://example.com/"+strings.Repeat("a", MaxLength)); err == nil || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("full URL not bounded: %v", err)
	}
	for _, base := range []string{"", "file:///tmp/scene", "https://user:password@example.com", "http://example.com?x=1", "http://example.com#v1=ignored"} {
		if _, err := EncodeAt(document, base); err == nil {
			t.Errorf("invalid base accepted: %s", base)
		}
	}
}
