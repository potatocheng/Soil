package main

import (
	_ "embed"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"text/template"
)

type studentInfo struct {
	Name    string
	Country string
}

type student struct {
	Id uint64
	studentInfo
}

//go:embed template.gohtml
var genOrm string

func gen(w io.Writer, filename string) error {
	fileSet := token.NewFileSet()
	f, err := parser.ParseFile(fileSet, filename, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	visitor := &SingleFileEntryVisitor{}
	ast.Walk(visitor, f)
	fileInfo := visitor.Get()

	//读取模板文件
	tpl, err := template.New("gen-orm").Parse(genOrm)
	if err != nil {
		return err
	}
	if err = tpl.Execute(w, fileInfo); err != nil {
		return err
	}

	return nil
}

func main() {
	//name := "cats"
	//strTemplate := "I like {{.}}!\n"
	//templ, err := template.New("templateName").Parse(strTemplate) //解析模板
	//if err != nil {
	//	panic(err)
	//}
	//err = templ.Execute(os.Stdout, name) //将name值填充到模板中
	//
	//yi := student{Id: 1234, studentInfo: studentInfo{Name: "yi", Country: "china"}}
	//strTemplate = "My name is {{.Name}}[id: {{.Id}}] from {{.Country}}\n"
	//templ, err = template.New("templateStruct").Parse(strTemplate)
	//if err != nil {
	//	panic(err)
	//}
	//err = templ.Execute(os.Stdout, yi)
	//
	//yang := student{Id: 1234, studentInfo: studentInfo{Name: "yang", Country: "China"}}
	//strTemplate = "my name is {{.Name}}[{{.Id}}] from {{if eq .Name `yi`}}{{.Country}}{{else}}Chinese{{end}}\n"
	//templ, err = template.New("template_if").Parse(strTemplate)
	//if err != nil {
	//	panic(err)
	//}
	//err = templ.Execute(os.Stdout, yi)
	//err = templ.Execute(os.Stdout, yang)
	//
	//students := make([]student, 0, 2)
	//students = append(students, yi)
	//students = append(students, yang)
	//strTemplate = "{{range.}}My name is {{.Name}}[id: {{.Id}}] from {{.Country}}\n{{end}}"
	//templ, err = template.New("template_range").Parse(strTemplate)
	//if err != nil {
	//	panic(err)
	//}
	//err = templ.Execute(os.Stdout, students)
}
