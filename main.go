package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"strconv"
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
	case *ast.BasicLit:
		if r.Kind == token.INT {
			return r.Value
		}
	case *ast.Ident:
		switch r.Name {
		case "string":
			return "String"
		case "int":
			return "i64"
		case "int64":
			return "i64"
		case "uint64":
			return "u64"
		default:
			return r.Name
		}
	case *ast.ArrayType:
		if r.Len != nil {
			// tuple
			n, _ := strconv.ParseInt(go2rusttype(r.Len), 10, 64)
			// fuck errors
			what := fmt.Sprintf("%s, ", go2rusttype(r.Elt))
			repeated := strings.Repeat(what, int(n))
			return fmt.Sprintf("(%s)", strings.TrimRight(repeated, ", "))
		} else {
			// slice
			return fmt.Sprintf("Vec<%s>", go2rusttype(r.Elt))
		}
	case *ast.StarExpr:
		return go2rusttype(r.X)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s::%s", go2rusttype(r.X), r.Sel.Name)
	case *ast.MapType:
		keytype := go2rusttype(r.Key)
		valtype := go2rusttype(r.Value)
		return fmt.Sprintf("HashMap<%s, %s>", keytype, valtype)
	case *ast.StructType:
		return "HashMap<String, String>"
	default:
		// nested structs are left
		// type Blah struct {
		//	 HostConfig struct {
		//     NetworkMode string `json:",omitempty"`
		// 	 }
		// }
		return fmt.Sprintf("FUCK YOU, %s", reflect.TypeOf(r))
	}

	return "OH BOY WE SCREWED THE POOCH HERE"
}

func printInner(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.StructType:
		for _, el := range v.Fields.List {
			rusttype := go2rusttype(el.Type)
			rename, optional := parseTag(el.Tag)
			name := ToSnakeCase(rusttype)
			needsFlatten := false
			if len(el.Names) > 0 {
				name = ToSnakeCase(el.Names[0].Name)
				if len(rename) == 0 {
					rename = el.Names[0].Name
				}
			} else {
				rename = rusttype
				needsFlatten = true
			}
			if optional {
				rusttype = fmt.Sprintf("Option<%s>", rusttype)
			}
			if needsFlatten {
				fmt.Printf("    #[serde(flatten)]\n")
			}
			if rename != "" {
				fmt.Printf("    #[serde(rename=\"%s\")]\n", rename)
			}
			name = ToSnakeCase(name)
			if name == "type" {
				name = "kind"
			}
			fmt.Printf("    pub %s: %s,\n", name, rusttype)
		}
	default:
		return false
	}
	return false
}

func main() {
	fname := os.Args[1] + ".go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fname, nil, parser.ParseComments)
	if err != nil {
		println("Unable to parse:" + err.Error())
		return
	}
	fmt.Println("// GENERATED! DO NOT MANGLE. source:", fname)
	fmt.Println("use serde;")
	fmt.Println("use serde_derive::{Serialize, Deserialize};")
	fmt.Print("\n\n")
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Comment:
			fmt.Println(v.Text)
		case *ast.TypeSpec:
			fmt.Println("#[derive(Serialize, Deserialize, Debug, Clone)]")
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
