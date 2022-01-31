package utils

import (
	"fmt"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func JsonEncodeBytes(b []byte) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf(`"%s"`, b))}
}
