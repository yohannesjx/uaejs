package service

import "bytes"

// noopReader wraps a byte slice as an io.Reader. Used internally so that
// hashed bytes can be re-read for the storage upload without a second
// io.ReadAll call.
func noopReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
