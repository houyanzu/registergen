package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var MODULE string
var imports []string
var inits []string

func main() {
	var root string
	flag.StringVar(&root, "root", "app/api/home/", "root dir")
	flag.Parse()

	var err error
	MODULE, err = GetModuleName()
	if err != nil {
		fmt.Println("Error GetModuleName:", err)
		return
	}

	// 打开或创建 register.go 文件
	file, err := os.Create(root + "register.go")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()
	str := `// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.` + "\n\n"

	_, err = file.WriteString(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	// 写入 package 声明
	_, err = file.WriteString("package main\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	imports = make([]string, 0)
	inits = make([]string, 0)

	err = scanDirectories(root, file)
	if err != nil {
		fmt.Println("Error scanning directories:", err)
		return
	}

	// 写入导入语句
	importStr := `import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"github.com/gin-gonic/gin"
	"github.com/houyanzu/work-box/tool/middleware"
`
	for _, v := range imports {
		importStr += "\t" + v + "\n"
	}
	importStr += ")\n\n"
	_, err = file.WriteString(importStr)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	// 写入函数定义
	_, err = file.WriteString("var controllers []interface{}\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	_, err = file.WriteString("func RegisterController(controller interface{}) {\n\tcontrollers = append(controllers, controller)\n}\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	ss := ""
	for _, v := range inits {
		// 写入函数定义
		ss += v
	}
	ss = "func init() {" + ss + "\n}\n"
	_, err = file.WriteString(ss)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	otherStr := `
func Register(router *gin.Engine) {
	for _, v := range controllers {
		AutoRegisterRoutes(router, v)
	}
}

// 自动注册路由
func AutoRegisterRoutes(router *gin.Engine, controller interface{}) {
	controllerType := reflect.TypeOf(controller)
	controllerValue := reflect.ValueOf(controller)

	// 获取基础路由前缀
	baseRoute := buildBaseRoute(controllerType)

	// 遍历控制器的所有方法并注册路由
	for i := 0; i < controllerType.NumMethod(); i++ {
		method := controllerType.Method(i)
		methodName := getControllerRouterName(method.Name)
		methodType := method.Type
		numIn := methodType.NumIn()
		if numIn < 2 {
			continue
		}

		firstParamType := getParamTypeName(methodType.In(1))
		if firstParamType != "Context" {
			continue
		}

		// 注册方法为 Gin 的 Post 路由
		route := fmt.Sprintf("api/%s/%s", baseRoute, methodName)
		switch numIn {
		case 2:
			router.POST(route, func(ctx *gin.Context) {
				method.Func.Call([]reflect.Value{controllerValue, reflect.ValueOf(ctx)})
			})

		case 3:
			secondParamType := getParamTypeName(methodType.In(2))
			if secondParamType == "uint" {
				router.POST(route, middleware.Login(), func(ctx *gin.Context) {
					userID := middleware.GetUserId(ctx)
					method.Func.Call([]reflect.Value{controllerValue, reflect.ValueOf(ctx), reflect.ValueOf(userID)})
				})
			}
		}

	}
}

// 构建控制器的基础路由前缀
func buildBaseRoute(controllerType reflect.Type) string {
	// 解析包路径
	pkgPath := controllerType.PkgPath()
	pkgParts := strings.Split(pkgPath, "/")

	// 构建路由路径
	var routeBuilder strings.Builder
	flag := false

	for _, part := range pkgParts {
		if part == "controller" {
			break
		}
		if flag {
			routeBuilder.WriteString(part + "/")
		}
		if part == "home" {
			flag = true
		}
	}

	// 添加控制器名
	controllerName := getControllerRouterName(controllerType.Name())
	routeBuilder.WriteString(controllerName)

	return routeBuilder.String()
}

// 去掉字符串末尾的 "Controller" 并将首字母变为小写
func getControllerRouterName(input string) string {
	input = strings.TrimSuffix(input, "Controller")
	if input == "" {
		return input
	}

	runes := []rune(input)
	runes[0] = unicode.ToLower(runes[0])

	return string(runes)
}

func getParamTypeName(paramType reflect.Type) string {
	if paramType.Kind() == reflect.Ptr {
		return paramType.Elem().Name()
	} else {
		return paramType.Name()
	}
}

`
	_, err = file.WriteString(otherStr)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	fmt.Println("Register file generated successfully.")
}

// 递归遍历目录，处理 controller 目录中的 Go 文件
func scanDirectories(root string, file *os.File) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && d.Name() == "controller" {
			// 遇到 controller 目录，处理其中的 Go 文件
			return filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
					return processGoFile(path, file)
				}
				return nil
			})
		}
		return nil
	})
}

// 处理 Go 文件中的控制器
func processGoFile(filePath string, file *os.File) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return err
	}

	pak := getImportPkg(filePath)
	alias := fmt.Sprintf("controller%d", len(imports))
	have := false
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if isControllerType(typeSpec.Name.Name) {
				controllerName := typeSpec.Name.Name
				controllerName = alias + "." + controllerName
				// Write out code to register the controller
				inits = append(inits, fmt.Sprintf("\n\tRegisterController(%s{})", controllerName))

				have = true
			}
		}
	}
	if have {
		imports = append(imports, alias+" \""+pak+"\"")
	}

	return nil
}

// 检查结构体名是否以 "Controller" 结尾
func isControllerType(name string) bool {
	return strings.HasSuffix(name, "Controller")
}

// GetModuleName 从 go.mod 文件中读取模块名称
func GetModuleName() (string, error) {
	// 打开 go.mod 文件
	file, err := os.Open("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer file.Close()

	// 创建读取器
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read from go.mod")
	}

	// 读取第一行
	line := scanner.Text()
	if !strings.HasPrefix(line, "module ") {
		return "", fmt.Errorf("first line does not start with 'module '")
	}

	// 返回模块名称
	moduleName := strings.TrimPrefix(line, "module ")
	return moduleName, nil
}

// 修改文件路径
func getImportPkg(filePath string) string {
	// 将反斜杠替换为正斜杠
	filePath = strings.ReplaceAll(filePath, "\\", "/")

	// 拼接 MODULE 和文件路径
	updatedPath := MODULE + "/" + filePath

	// 找到最后一个 "/" 的位置
	lastSlashIndex := strings.LastIndex(updatedPath, "/")
	if lastSlashIndex != -1 {
		// 去掉最后一个 "/" 以及其后面的部分
		updatedPath = updatedPath[:lastSlashIndex]
	}

	return updatedPath
}
