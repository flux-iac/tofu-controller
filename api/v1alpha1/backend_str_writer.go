package v1alpha1

import (
	"bytes"
	"fmt"
)

type writer struct {
	buf *bytes.Buffer
}

func newWriter() *writer {
	return &writer{buf: &bytes.Buffer{}}
}

func (w *writer) W(format string, args ...interface{}) {
	w.buf.WriteString(fmt.Sprintf(format, args...) + "\n")
}

func (w *writer) String() string {
	return w.buf.String()
}
