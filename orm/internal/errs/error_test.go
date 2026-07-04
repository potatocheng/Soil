package errs

import (
	"errors"
	"strings"
	"testing"
)

// TestSentinelErrorsAreMatchable 验证各 sentinel 错误可被 errors.Is 直接匹配。
func TestSentinelErrorsAreMatchable(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrNoRows", ErrNoRows},
		{"ErrTooManyRows", ErrTooManyRows},
		{"ErrInvalidColumn", ErrInvalidColumn},
		{"ErrUnsupportedFeature", ErrUnsupportedFeature},
		{"ErrNilPointer", ErrNilPointer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.err) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tc.err, tc.err)
			}
		})
	}
}

// TestNewErrInvalidColumnWrapsSentinel 验证 NewErrInvalidColumn 返回的错误
// 可通过 errors.Is 匹配到 ErrInvalidColumn sentinel。
func TestNewErrInvalidColumnWrapsSentinel(t *testing.T) {
	err := NewErrInvalidColumn("foo")
	if err == nil {
		t.Fatal("NewErrInvalidColumn returned nil")
	}
	if !errors.Is(err, ErrInvalidColumn) {
		t.Fatalf("errors.Is(err, ErrInvalidColumn) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "foo")
	}
}

// TestNewErrUnsupportedFeatureWrapsSentinel 验证 NewErrUnsupportedFeature 返回的错误
// 可通过 errors.Is 匹配到 ErrUnsupportedFeature sentinel。
func TestNewErrUnsupportedFeatureWrapsSentinel(t *testing.T) {
	err := NewErrUnsupportedFeature("upsert")
	if err == nil {
		t.Fatal("NewErrUnsupportedFeature returned nil")
	}
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("errors.Is(err, ErrUnsupportedFeature) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), "upsert") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "upsert")
	}
}

// TestNewErrInvalidColumnDifferentInstances 验证不同入参构造的错误互不相同，
// 但都能匹配到同一个 sentinel。
func TestNewErrInvalidColumnDifferentInstances(t *testing.T) {
	a := NewErrInvalidColumn("col_a")
	b := NewErrInvalidColumn("col_b")
	if errors.Is(a, b) {
		t.Fatalf("different wrapped errors should not match each other")
	}
	if !errors.Is(a, ErrInvalidColumn) || !errors.Is(b, ErrInvalidColumn) {
		t.Fatalf("both should match ErrInvalidColumn sentinel")
	}
}
