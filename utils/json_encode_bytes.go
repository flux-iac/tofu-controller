package utils

import (
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// JSONEncodeBytes encodes the given byte slice to a JSON byte slice.
func JSONEncodeBytes(b []byte) (*apiextensionsv1.JSON, error) {
	data, err := json.Marshal(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not encode bytes to json: %w", err)
	}
	return &apiextensionsv1.JSON{Raw: data}, nil
}

// MustJSONEncodeBytes is like JSONEncodeBytes but panics on error.
func MustJSONEncodeBytes(b []byte) *apiextensionsv1.JSON {
	data, err := json.Marshal(string(b))
	if err != nil {
		panic(err)
	}
	return &apiextensionsv1.JSON{Raw: data}
}
