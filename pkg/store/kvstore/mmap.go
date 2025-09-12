package kvstore

import "io"

// mmapReader implements io.Reader for memory-mapped data
type mmapReader struct {
	data   []byte
	offset int
}

func (r *mmapReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
