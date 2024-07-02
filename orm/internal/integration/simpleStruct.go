package integration

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/ecodeclub/ekit"
)

type SimpleStruct struct {
	Id      uint64
	Bool    bool
	BoolPtr *bool

	Int    int
	IntPtr *int

	Int8    int8
	Int8Ptr *int8

	Int16    int16
	Int16Ptr *int16

	Int32    int32
	Int32Ptr *int32

	Int64    int64
	Int64Ptr *int64

	Uint    uint
	UintPtr *uint

	Uint8    uint8
	Uint8Ptr *uint8

	Uint16    uint16
	Uint16Ptr *uint16

	Uint32    uint32
	Uint32Ptr *uint32

	Uint64    uint64
	Uint64Ptr *uint64

	Float32    float32
	Float32Ptr *float32

	Float64    float64
	Float64Ptr *float64

	Byte      byte
	BytePtr   *byte
	ByteArray []byte

	String string

	NullStringPtr  *sql.NullString
	NullInt16Ptr   *sql.NullInt16
	NullInt32Ptr   *sql.NullInt32
	NullInt64Ptr   *sql.NullInt64
	NullBoolPtr    *sql.NullBool
	NullFloat64Ptr *sql.NullFloat64
	JsonColumn     *JsonColumn
}

type User struct {
	Name string
	Age  uint32
}

type JsonColumn struct {
	Val   User
	Valid bool
}

// Scan 允许你将数据库中的数据直接映射到自定义的数据类型
// 在值接收器和指针接收器上都有方法。 Go 文档不推荐这种用法。但是NullString中就是这样写的
func (jc *JsonColumn) Scan(value any) error {
	if value == nil {
		jc.Valid = false
		return nil
	}

	var bs []byte
	switch v := value.(type) {
	case []byte:
		bs = v
	case string:
		bs = []byte(v)
	case *[]byte:
		bs = *v
	default:
		return fmt.Errorf("不合法类型 %+v", value)
	}

	if len(bs) == 0 {
		jc.Valid = false
		return nil
	}

	err := json.Unmarshal(bs, &jc.Val)
	if err != nil {
		return err
	}

	jc.Valid = true
	return nil
}

func (jc JsonColumn) Value() (driver.Value, error) {
	if !jc.Valid {
		return nil, nil
	}
	// 将struct序列化为json
	bs, err := json.Marshal(jc.Val)
	if err != nil {
		return nil, err
	}

	return bs, nil
}

func NewSimpleStruct(id uint64) *SimpleStruct {
	return &SimpleStruct{
		Id:             id,
		Bool:           true,
		BoolPtr:        ekit.ToPtr[bool](false),
		Int:            12,
		IntPtr:         ekit.ToPtr[int](13),
		Int8:           8,
		Int8Ptr:        ekit.ToPtr[int8](-8),
		Int16:          16,
		Int16Ptr:       ekit.ToPtr[int16](-16),
		Int32:          32,
		Int32Ptr:       ekit.ToPtr[int32](-32),
		Int64:          64,
		Int64Ptr:       ekit.ToPtr[int64](-64),
		Uint:           14,
		UintPtr:        ekit.ToPtr[uint](15),
		Uint8:          8,
		Uint8Ptr:       ekit.ToPtr[uint8](18),
		Uint16:         16,
		Uint16Ptr:      ekit.ToPtr[uint16](116),
		Uint32:         32,
		Uint32Ptr:      ekit.ToPtr[uint32](132),
		Uint64:         64,
		Uint64Ptr:      ekit.ToPtr[uint64](164),
		Float32:        3.2,
		Float32Ptr:     ekit.ToPtr[float32](-3.2),
		Float64:        6.4,
		Float64Ptr:     ekit.ToPtr[float64](-6.4),
		Byte:           byte(8),
		BytePtr:        ekit.ToPtr[byte](18),
		ByteArray:      []byte("hello"),
		String:         "world",
		NullStringPtr:  &sql.NullString{String: "null string", Valid: true},
		NullInt16Ptr:   &sql.NullInt16{Int16: 16, Valid: true},
		NullInt32Ptr:   &sql.NullInt32{Int32: 32, Valid: true},
		NullInt64Ptr:   &sql.NullInt64{Int64: 64, Valid: true},
		NullBoolPtr:    &sql.NullBool{Bool: true, Valid: true},
		NullFloat64Ptr: &sql.NullFloat64{Float64: 6.4, Valid: true},
		JsonColumn: &JsonColumn{
			Val:   User{Name: "Tom", Age: 18},
			Valid: true,
		},
	}
}
