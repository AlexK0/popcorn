package common

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc/encoding"
)

const ZstdName = "zstd"

type compressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

func init() {
	enc, _ := zstd.NewWriter(nil)
	dec, _ := zstd.NewReader(nil)
	c := &compressor{
		encoder: enc,
		decoder: dec,
	}
	encoding.RegisterCompressor(c)
}

func (c *compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return &zstdWriteCloser{
		enc:    c.encoder,
		writer: w,
	}, nil
}

type zstdWriteCloser struct {
	enc    *zstd.Encoder
	writer io.Writer
	buf    bytes.Buffer
}

func (z *zstdWriteCloser) Write(p []byte) (int, error) {
	return z.buf.Write(p)
}

func (z *zstdWriteCloser) Close() error {
	compressed := z.enc.EncodeAll(z.buf.Bytes(), nil)
	_, err := io.Copy(z.writer, bytes.NewReader(compressed))
	return err
}

func (c *compressor) Decompress(r io.Reader) (io.Reader, error) {
	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	uncompressed, err := c.decoder.DecodeAll(compressed, nil)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(uncompressed), nil
}

func (c *compressor) Name() string {
	return ZstdName
}
