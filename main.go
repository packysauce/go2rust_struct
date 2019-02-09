package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

var matchFirstCap = regexp.MustCompile("(.)([A-Z]([a-z])+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")
var pullTag = regexp.MustCompile("`json:\"(?P<name>[^,]*)(?P<option>,omitempty)?\"`")

func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.Replace(strings.ToLower(snake), "i_p", "ip", -1)
}

func parseTag(v *ast.BasicLit) (string, bool) {
	if v == nil {
		return "", false
	}
	rename := ""
	optional := false
	match := pullTag.FindStringSubmatch(v.Value)
	for i, name := range pullTag.SubexpNames() {
		if name == "name" && string(match[i]) != "" {
			rename = string(match[i])
		}
		if name == "option" && string(match[i]) == ",omitempty" {
			optional = true
		}
	}

	return rename, optional
}

func go2rusttype(n ast.Node) string {
	switch r := n.(type) {
	case *ast.Ident:
		switch r.Name {
		case "string":
			return "String"
		case "int":
			return "i64"
		default:
			return r.Name
		}
	case *ast.StarExpr:
		return go2rusttype(r.X)
	}
	return "fuck you we made it right here"
}

func printInner(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.StructType:
		for _, el := range v.Fields.List {
			var rusttype string
			switch r := el.Type.(type) {
			case *ast.Ident:
				rusttype = go2rusttype(r)
			case *ast.MapType:
				keytype := go2rusttype(r.Key)
				valtype := go2rusttype(r.Value)
				rusttype = fmt.Sprintf("HashMap<%s, %s>", keytype, valtype)
			case *ast.ArrayType:
				rusttype = fmt.Sprintf("Vec<%s>", go2rusttype(r.Elt))
			case *ast.StarExpr:
				rusttype = go2rusttype(r.X)
			default:
				rusttype = "<unknown>"
			}
			rename, optional := parseTag(el.Tag)
			name := ToSnakeCase(el.Names[0].Name)
			if len(rename) == 0 {
				rename = el.Names[0].Name
			}
			if optional {
				rusttype = fmt.Sprintf("Option<%s>", rusttype)
			}
			fmt.Printf("    #[serde(rename=%s)]\n", rename)
			fmt.Printf("    pub %s: %s,\n", ToSnakeCase(name), rusttype)
		}
	default:
		return false
	}
	return false
}

func main() {
	fname := os.Args[1] + ".go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fname, nil, 0)
	if err != nil {
		println("Unable to parse:" + err.Error())
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.TypeSpec:
			fmt.Println("#[derive(Serialize, Deserialize, Debug, Copy, Clone)]")
			fmt.Println("pub struct", v.Name.Name, "{")
			ast.Inspect(v.Type, printInner)
			fmt.Println("}\n\n")
			return false
		case nil:
			return false
		default:
			return true
		}
		return true
	})
}
