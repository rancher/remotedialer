package remotedialer

import "io"

type byteReader struct {
	io.Reader
}

func (b *byteReader) ReadByte() (byte, error) {
	singleByte := make([]byte, 1)
	n, err := b.Read(singleByte)
	if n == 0 {
		return 0, err
	}
	return singleByte[0], err
}

func (b *byteReader) ReadBytes(n int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(b, n))
}
