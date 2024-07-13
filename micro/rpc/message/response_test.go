package message

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDecodeAndEncodeResponse(t *testing.T) {
	testCases := []struct {
		name     string
		response *Response
	}{
		{
			name: "no error, no data",
			response: &Response{
				MessageID:  267,
				Version:    1,
				Compressor: 2,
				Serializer: 3,
			},
		},
		{
			name: "no data",
			response: &Response{
				MessageID:  267,
				Version:    1,
				Compressor: 2,
				Serializer: 3,
				ErrorInfo:  []byte("this is an error"),
			},
		},
		{
			name: "no Error",
			response: &Response{
				MessageID:  267,
				Version:    1,
				Compressor: 2,
				Serializer: 3,
				Data:       []byte("hello response"),
			},
		},
		{
			name: "normal",
			response: &Response{
				MessageID:  267,
				Version:    1,
				Compressor: 2,
				Serializer: 3,
				ErrorInfo:  []byte("this is an error"),
				Data:       []byte("hello response"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.response.CalculateHeadLength()
			tc.response.CalculateBodyLength()
			data := EncodeResponse(tc.response)
			resp := DecodeResponse(data)
			assert.Equal(t, tc.response, resp)
		})
	}
}
