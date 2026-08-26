package io

import (
	"bytes"
	"io"
)

// newTrimReader wraps raw script bytes, stripping a UTF-8 byte order mark so a
// file saved by a Windows editor still parses.
func newTrimReader(data []byte) io.Reader {
	return bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
}
