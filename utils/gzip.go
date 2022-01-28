package utils

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
)

func GzipEncode(tfplan []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)

	_, err := w.Write(tfplan)
	if err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func GzipDecode(encodedPlan []byte) ([]byte, error) {
	re := bytes.NewReader(encodedPlan)
	gr, err := gzip.NewReader(re)
	if err != nil {
		return nil, err
	}

	o, err := ioutil.ReadAll(gr)
	if err != nil {
		return nil, err
	}

	if err = gr.Close(); err != nil {
		return nil, err
	}
	return o, nil
}
