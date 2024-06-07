package model

import (
	"Soil/orm/internal/errs"
	"bytes"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

type Registry interface {
	// Get 获得val对应的模型，如果没有这个模型，调用Registry创建一个模型缓存在registry中
	Get(val any) (*Model, error)
	// Registry 注册一个模型
	Registry(val any, opts ...ModelOpt) (*Model, error)
}

// registry 作为一个缓存，缓存数据表元数据
type registry struct {
	lock   sync.RWMutex //读写锁
	models map[reflect.Type]*Model
}

func NewRegistry() Registry {
	return &registry{
		models: make(map[reflect.Type]*Model),
	}
}

// Get 接收一级指针
func (r *registry) Get(entity any) (*Model, error) {
	// 第一次检查: 通过读取锁来检查数据是否已经存在。如果存在直接返回结果，避免不必要的写锁开销
	r.lock.RLock()
	typ := reflect.TypeOf(entity)
	m, ok := r.models[typ] // 找到go结构体对应的数据表元数据
	r.lock.RUnlock()
	if ok {
		return m, nil
	}
	// 第二次检查: 如果第一次检查未找到数据，在后去写锁后再次检查。
	r.lock.Lock()
	defer r.lock.Unlock()
	m, ok = r.models[typ]
	if ok {
		return m, nil
	}
	// 如果没有将entity解析成对应的数据表元数据，先解析缓存下来
	var err error
	if m, err = r.Registry(entity); err != nil {
		return nil, err
	}

	r.models[typ] = m
	return m, nil
}

func (r *registry) Registry(val any, opts ...ModelOpt) (*Model, error) {
	m, err := r.parseModel(val)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		err = opt(m)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// 将结构体转换为对应数据表的元数据，例如，结构体名对应表名
func (r *registry) parseModel(entity any) (*Model, error) {
	typ := reflect.TypeOf(entity)
	//只接受一级指针
	if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
		return nil, errs.ErrPointerOnly
	}
	typ = typ.Elem()
	//处理列名
	numField := typ.NumField()
	fields := make(map[string]*Field, numField)
	columns := make(map[string]*Field, numField)
	fds := make([]*Field, numField)
	for i := 0; i < numField; i++ {
		f := typ.Field(i)
		tags, err := r.parseTag(f.Tag) //如果有tag列名按照tag设置
		if err != nil {
			return nil, err
		}
		colName := tags[tagKeyColumn]
		if colName == "" {
			//没有指定列名，对列名默认驼峰转下划线
			colName = Camel2Case(f.Name)
		}
		fieldMeta := &Field{ColName: colName, Type: f.Type, GoName: f.Name, Offset: f.Offset}
		fields[f.Name] = fieldMeta
		columns[colName] = fieldMeta
		fds[i] = fieldMeta
	}
	// 处理表名
	var tableName string
	if tn, ok := entity.(TableName); ok {
		tableName = tn.TableName()
	}

	if tableName == "" {
		tableName = Camel2Case(typ.Name())
	}
	return &Model{TableName: tableName, FieldMap: fields, ColumnMap: columns, Fields: fds}, nil
}

// "orm:column(user_t);size(60)"
func (r *registry) parseTag(tag reflect.StructTag) (map[string]string, error) {
	val, ok := tag.Lookup("orm")
	if !ok {
		return map[string]string{}, nil
	}

	res := make(map[string]string)
	segments := strings.Split(val, ";")
	for _, seg := range segments {
		if !strings.HasSuffix(seg, ")") {
			return nil, errs.NewErrInvalidTagContent(seg)
		}

		pair := strings.Split(seg, "(")
		if len(pair) != 2 {
			return nil, errs.NewErrInvalidTagContent(seg)
		}

		pair[1] = strings.TrimSuffix(pair[1], ")")

		res[pair[0]] = pair[1]
	}

	return res, nil
}

// Camel2Case 驼峰转下划线
func Camel2Case(str string) string {
	buffer := bytes.Buffer{}
	for i, r := range str {
		if unicode.IsUpper(r) {
			if i != 0 {
				buffer.WriteString("_")
			}
			buffer.WriteRune(unicode.ToLower(r))
		} else {
			buffer.WriteRune(r)
		}
	}
	return buffer.String()
}

func WithTableName(tableName string) ModelOpt {
	return func(m *Model) error {
		m.TableName = tableName
		return nil
	}
}

func WithColumName(field string, colName string) ModelOpt {
	return func(m *Model) error {
		fd, ok := m.FieldMap[field]
		if !ok {
			return errs.NewErrUnknownField(field)
		}

		oldColField, ok := m.ColumnMap[fd.ColName]
		if !ok {
			return errs.NewErrUnknownColumn(field)
		}
		delete(m.ColumnMap, fd.ColName)
		oldColField.ColName = colName
		m.ColumnMap[colName] = oldColField

		fd.ColName = colName

		return nil
	}
}
