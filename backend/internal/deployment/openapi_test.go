package deployment

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths map[string]map[string]struct {
			ID      string `yaml:"operationId"`
			Status  string `yaml:"x-implementation-status"`
			Servers []struct {
				URL string `yaml:"url"`
			} `yaml:"servers"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	documented := make(map[string]bool)
	identifiers := make(map[string]bool)
	for path, methods := range contract.Paths {
		for method, operation := range methods {
			key := strings.ToUpper(method) + " " + path
			if operation.Status != "implemented" || operation.ID == "" || identifiers[operation.ID] {
				t.Errorf("%s: missing implementation status or unique operation ID", key)
			}
			identifiers[operation.ID] = true
			documented[key] = true
			if path == "/metrics" && (len(operation.Servers) != 1 || operation.Servers[0].URL != "/") {
				t.Error("metrics must override /api/v1 with the internal backend root")
			}
		}
	}
	registered := make(map[string]bool)
	for _, file := range []string{"../handler/router.go", "../../cmd/api/main.go"} {
		source, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(source, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			path, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Error(err)
				return true
			}
			method := selector.Sel.Name
			if method == "Handle" && path == "/metrics" {
				method = "GET"
			}
			switch method {
			case "GET", "POST", "PATCH", "PUT", "DELETE", "HEAD":
			default:
				return true
			}
			if group, ok := selector.X.(*ast.Ident); ok && group.Name == "external" {
				path = "/integration" + path
			}
			parts := strings.Split(path, "/")
			for i, part := range parts {
				if strings.HasPrefix(part, ":") {
					parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
				}
			}
			registered[method+" "+strings.Join(parts, "/")] = true
			return true
		})
	}
	for key := range registered {
		if !documented[key] {
			t.Errorf("registered route missing from OpenAPI: %s", key)
		}
	}
	for key := range documented {
		if !registered[key] {
			t.Errorf("OpenAPI route is not registered: %s", key)
		}
	}
}
