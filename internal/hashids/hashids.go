// Package hashidlib encodes and decodes integer IDs as Hashid strings.
package hashidlib

import (
	"fmt"

	ghashids "github.com/speps/go-hashids/v2"
)

const (
	salt      = "tld-is-awesome"
	minLength = 8
)

var hd *ghashids.HashID

func init() {
	var err error
	data := ghashids.NewData()
	data.Salt = salt
	data.MinLength = minLength
	hd, err = ghashids.NewWithData(data)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize hashids: %v", err))
	}
}

// Encode converts an int32 ID to a Hashid string.
func Encode(id int32) string {
	if id == 0 {
		return ""
	}
	h, err := hd.Encode([]int{int(id)})
	if err != nil {
		return ""
	}
	return h
}

// Decode converts a Hashid string back to an int32 ID.
func Decode(h string) (int32, error) {
	if h == "" {
		return 0, nil
	}
	ids, err := hd.DecodeWithError(h)
	if err != nil {
		return 0, fmt.Errorf("decode hashid %q: %w", h, err)
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("invalid hashid: %s", h)
	}
	return int32(ids[0]), nil
}
