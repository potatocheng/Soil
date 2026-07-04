package model

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCamel2Case(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "single word lower",
			in:   "name",
			want: "name",
		},
		{
			name: "single word upper first",
			in:   "Name",
			want: "name",
		},
		{
			name: "consecutive upper case as one word",
			in:   "ID",
			want: "id",
		},
		{
			name: "IDName should be id_name",
			in:   "IDName",
			want: "id_name",
		},
		{
			name: "UserName should be user_name",
			in:   "UserName",
			want: "user_name",
		},
		{
			name: "HTTPServer should be http_server",
			in:   "HTTPServer",
			want: "http_server",
		},
		{
			name: "userID should be user_id",
			in:   "userID",
			want: "user_id",
		},
		{
			name: "simple CamelCase",
			in:   "FirstName",
			want: "first_name",
		},
		{
			name: "all upper HTTP",
			in:   "HTTP",
			want: "http",
		},
		{
			name: "HTTPSConn",
			in:   "HTTPSConn",
			want: "https_conn",
		},
		{
			name: "lower first with upper sequence",
			in:   "myIDName",
			want: "my_id_name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Camel2Case(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}
