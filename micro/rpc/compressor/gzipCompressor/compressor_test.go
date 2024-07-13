package gzipCompressor

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGzipCompressAndDeDecompress(t *testing.T) {
	testCases := []struct {
		name       string
		input      []byte
		wantResult []byte
	}{
		{
			name:       "normal",
			input:      []byte("hello world"),
			wantResult: []byte("hello world"),
		},
	}

	c := &Compressor{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cData, err := c.Compress(tc.input)
			require.NoError(t, err)
			res, err := c.Decompress(cData)
			require.NoError(t, err)
			require.Equal(t, tc.wantResult, res)
		})
	}
}
