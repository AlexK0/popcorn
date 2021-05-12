package common

import (
	"io"
	"io/ioutil"
	"sync"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc/encoding"
)

const ZstdCompressorName = "zstd"

func init() {
	c := &zstdCompressor{}
	c.poolCompressor.New = func() interface{} {
		zWriter, _ := zstd.NewWriter(ioutil.Discard, zstd.WithWindowSize(512*1024))
		return &writer{Encoder: zWriter, pool: &c.poolCompressor}
	}
	encoding.RegisterCompressor(c)
}

type writer struct {
	*zstd.Encoder
	pool *sync.Pool
}

func (c *zstdCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	z := c.poolCompressor.Get().(*writer)
	z.Encoder.Reset(w)
	return z, nil
}

func (z *writer) Close() error {
	defer z.pool.Put(z)
	return z.Encoder.Close()
}

type reader struct {
	*zstd.Decoder
	pool *sync.Pool
}

func (c *zstdCompressor) Decompress(r io.Reader) (io.Reader, error) {
	z, inPool := c.poolDecompressor.Get().(*reader)
	if !inPool {
		newZ, err := zstd.NewReader(r)
		if err != nil {
			return nil, err
		}
		return &reader{Decoder: newZ, pool: &c.poolDecompressor}, nil
	}
	if err := z.Reset(r); err != nil {
		c.poolDecompressor.Put(z)
		return nil, err
	}
	return z, nil
}

func (z *reader) Read(p []byte) (n int, err error) {
	n, err = z.Decoder.Read(p)
	if err == io.EOF {
		z.pool.Put(z)
	}
	return n, err
}

func (c *zstdCompressor) Name() string {
	return ZstdCompressorName
}

type zstdCompressor struct {
	poolCompressor   sync.Pool
	poolDecompressor sync.Pool
}
