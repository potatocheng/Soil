package errs

import (
	"errors"
	"strings"
	"testing"
)

// TestNewErrUnsupportedExpressionType 验证 NewErrUnsupportedExpressionType
// 返回非 nil 错误且包含入参信息。
func TestNewErrUnsupportedExpressionType(t *testing.T) {
	err := NewErrUnsupportedExpressionType(42)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "42")
	}
}

// TestNewErrInvalidTagContent 验证 NewErrInvalidTagContent 返回的错误包含入参。
func TestNewErrInvalidTagContent(t *testing.T) {
	err := NewErrInvalidTagContent("badsegment")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "badsegment") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "badsegment")
	}
}

// TestNewErrUnknownField 验证 NewErrUnknownField 返回的错误包含字段名。
func TestNewErrUnknownField(t *testing.T) {
	err := NewErrUnknownField("Foo")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "Foo") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "Foo")
	}
}

// TestNewErrUnknownColumn 验证 NewErrUnknownColumn 返回的错误包含列名。
func TestNewErrUnknownColumn(t *testing.T) {
	err := NewErrUnknownColumn("foo_col")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "foo_col") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "foo_col")
	}
}

// TestNewErrUnsupportedAssignableType 验证 NewErrUnsupportedAssignableType
// 返回的错误包含入参。
func TestNewErrUnsupportedAssignableType(t *testing.T) {
	err := NewErrUnsupportedAssignableType("weird-value")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "weird-value") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "weird-value")
	}
}

// TestNewErrFailToRollbackTx 验证 NewErrFailToRollbackTx 把业务错误以 %w 包装，
// 调用方可通过 errors.Is 匹配到原业务错误，且错误信息包含回滚错误与 panic 标记。
func TestNewErrFailToRollbackTx(t *testing.T) {
	bizErr := errors.New("biz boom")
	rbErr := errors.New("rollback boom")
	err := NewErrFailToRollbackTx(bizErr, rbErr, true)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	s := err.Error()
	for _, want := range []string{"biz boom", "rollback boom", "panic: true"} {
		if !strings.Contains(s, want) {
			t.Fatalf("err.Error() = %q, want it to contain %q", s, want)
		}
	}
	if !errors.Is(err, bizErr) {
		t.Fatalf("errors.Is(err, bizErr) = false, want true (bizErr should be wrapped)")
	}
}

// TestNewErrFailToRollbackTx_NoPanic 验证 panicked=false 时格式化输出 panic: false。
func TestNewErrFailToRollbackTx_NoPanic(t *testing.T) {
	err := NewErrFailToRollbackTx(errors.New("biz"), errors.New("rb"), false)
	if !strings.Contains(err.Error(), "panic: false") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "panic: false")
	}
}

// TestNewErrUnsupportedTable 验证 NewErrUnsupportedTable 返回的错误包含入参。
func TestNewErrUnsupportedTable(t *testing.T) {
	err := NewErrUnsupportedTable("someTableRef")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "someTableRef") {
		t.Fatalf("err.Error() = %q, want it to contain %q", err.Error(), "someTableRef")
	}
}

// TestExtraSentinelErrors 验证尚未在 error_test.go 中覆盖的 sentinel 错误
// 非空且可被 errors.Is 自身匹配。
func TestExtraSentinelErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrEntityNil", ErrEntityNil},
		{"ErrPointerOnly", ErrPointerOnly},
		{"ErrTooManyReturnedColumns", ErrTooManyReturnedColumns},
		{"ErrInsertZeroRow", ErrInsertZeroRow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			if !errors.Is(tc.err, tc.err) {
				t.Fatalf("errors.Is(%v, %v) = false", tc.err, tc.err)
			}
			if strings.TrimSpace(tc.err.Error()) == "" {
				t.Fatalf("%s has empty message", tc.name)
			}
		})
	}
}
