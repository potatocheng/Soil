package main

import (
	"go/ast"
)

type FileInfo struct {
	Package   string
	Imports   []string
	Variables map[string][]string
}

// SingleFileEntryVisitor 获取package内容
type SingleFileEntryVisitor struct {
	file *fileVisitor
}

func (s *SingleFileEntryVisitor) Visit(node ast.Node) (w ast.Visitor) {
	n, ok := node.(*ast.File)
	if ok {
		s.file = &fileVisitor{
			Package: n.Name.Name,
		}
		return s.file
	}
	return s
}

func (s *SingleFileEntryVisitor) Get() *FileInfo {
	if s.file != nil {
		return s.file.Get()
	}
	return nil
}

// fileVisitor 获取import的内容
type fileVisitor struct {
	Package      string
	Imports      []string
	typeVisitors []*typeVisitor
}

func (f *fileVisitor) Get() *FileInfo {
	return &FileInfo{
		Imports: f.Imports,
		Package: f.Package,
	}
}

func (f *fileVisitor) Visit(node ast.Node) (w ast.Visitor) {
	switch n := node.(type) {
	case *ast.ImportSpec:
		path := n.Path.Value
		if n.Name != nil && n.Name.Name != "" {
			path = n.Name.Name + " " + path
		}
		f.Imports = append(f.Imports, path)
	case *ast.TypeSpec:
		typVisitor := &typeVisitor{
			Name: n.Name.Name,
		}
		f.typeVisitors = append(f.typeVisitors, typVisitor)
		return typVisitor
	}

	return f
}

type typeVisitor struct {
	Name      string
	Variables []string
}

func (t *typeVisitor) Visit(node ast.Node) (w ast.Visitor) {
	n, ok := node.(*ast.Field)
	if ok {
		for _, name := range n.Names {
			t.Variables = append(t.Variables, name.Name)
		}
	}

	return t
}
