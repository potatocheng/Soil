package net

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEndpointList(t *testing.T) {
	eps, err := parseEndpointList("127.0.0.1:8080,127.0.0.1:8081@3; 10.0.0.1:9000")
	require.NoError(t, err)
	require.Len(t, eps, 3)
	assert.Equal(t, "127.0.0.1:8080", eps[0].Addr)
	assert.Equal(t, 1, eps[0].Weight)
	assert.Equal(t, "127.0.0.1:8081", eps[1].Addr)
	assert.Equal(t, 3, eps[1].Weight)
	assert.Equal(t, "10.0.0.1:9000", eps[2].Addr)
}

func TestParseEndpointListDedupe(t *testing.T) {
	eps, err := parseEndpointList("127.0.0.1:8080@1,127.0.0.1:8080@5")
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, 5, eps[0].Weight)
}

func TestParseEndpointListInvalid(t *testing.T) {
	_, err := parseEndpointList("not-an-addr")
	require.Error(t, err)
	_, err = parseEndpointList("127.0.0.1:8080@x")
	require.Error(t, err)
}

func TestDNSResolverIPPassthrough(t *testing.T) {
	r := &DNSResolver{}
	addrs, err := r.Resolve(context.Background(), "tcp", "127.0.0.1:8080")
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1:8080"}, addrs)
}

type fakeResolver struct {
	m map[string][]string
}

func (f fakeResolver) Resolve(_ context.Context, _, address string) ([]string, error) {
	if v, ok := f.m[address]; ok {
		return v, nil
	}
	return []string{address}, nil
}

func TestResolveEndpointsExpand(t *testing.T) {
	r := fakeResolver{m: map[string][]string{
		"svc.local:8080": {"10.0.0.1:8080", "10.0.0.2:8080"},
	}}
	out, err := resolveEndpoints(context.Background(), "tcp",
		[]Endpoint{{Addr: "svc.local:8080", Weight: 2}}, r)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, 2, out[0].Weight)
	assert.Equal(t, 2, out[1].Weight)
}
