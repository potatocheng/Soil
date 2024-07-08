package message

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDecodeAndEncodeRequest(t *testing.T) {
	testCases := []struct {
		name    string
		request *Request
	}{
		{
			name: "normal",
			request: &Request{
				MessageID:   478,
				Version:     1,
				Compressor:  2,
				Serializer:  3,
				ServiceName: "user-service",
				MethodName:  "GetById",
				Meta: map[string]string{
					"trace-id": "123467",
					"a/b":      "a",
				},
				Data: []byte("Hello, World"),
			},
		},
		{
			name: "without data",
			request: &Request{
				MessageID:   478,
				Version:     1,
				Compressor:  2,
				Serializer:  3,
				ServiceName: "user-service",
				MethodName:  "GetById",
				Meta: map[string]string{
					"trace-id": "123467",
					"a/b":      "a",
				},
			},
		},
		{
			name: "don`t have Meta with data",
			request: &Request{
				MessageID:   478,
				Version:     1,
				Compressor:  2,
				Serializer:  3,
				ServiceName: "user-service",
				MethodName:  "GetById",
				Data:        []byte("Hello, World"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.request.CalculateHeadLength()
			tc.request.CalculateBodyLength()
			data := EncodeRequest(tc.request)
			req := DecodeRequest(data)
			assert.Equal(t, tc.request, req)
		})
	}
}
