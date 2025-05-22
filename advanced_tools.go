package aistudio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// AdvancedToolsRegistry contains all the advanced tools for aistudio
type AdvancedToolsRegistry struct {
	toolManager *ToolManager
}

// NewAdvancedToolsRegistry creates a new registry and registers all advanced tools
func NewAdvancedToolsRegistry(tm *ToolManager) (*AdvancedToolsRegistry, error) {
	registry := &AdvancedToolsRegistry{toolManager: tm}
	
	// Register all advanced tools
	if err := registry.registerAllTools(); err != nil {
		return nil, fmt.Errorf("failed to register advanced tools: %w", err)
	}
	
	return registry, nil
}

// registerAllTools registers all the advanced tools with the tool manager
func (r *AdvancedToolsRegistry) registerAllTools() error {
	tools := []struct {
		name        string
		description string
		parameters  json.RawMessage
		handler     func(context.Context, map[string]interface{}) (interface{}, error)
	}{
		{
			"code_analyzer",
			"Analyze Go code files for complexity, dependencies, and potential issues",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {
						"type": "string",
						"description": "Path to the Go source file to analyze"
					},
					"analysis_type": {
						"type": "string",
						"enum": ["complexity", "dependencies", "style", "security", "all"],
						"default": "all",
						"description": "Type of analysis to perform"
					}
				},
				"required": ["file_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleCodeAnalyzer(argsJSON)
			},
		},
		{
			"test_generator",
			"Generate comprehensive unit tests for Go functions and methods",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"source_file": {
						"type": "string",
						"description": "Path to the Go source file to generate tests for"
					},
					"function_name": {
						"type": "string",
						"description": "Specific function to generate tests for (optional)"
					},
					"test_types": {
						"type": "array",
						"items": {
							"type": "string",
							"enum": ["unit", "benchmark", "fuzz", "integration"]
						},
						"default": ["unit"],
						"description": "Types of tests to generate"
					},
					"coverage_target": {
						"type": "number",
						"minimum": 0,
						"maximum": 100,
						"default": 80,
						"description": "Target code coverage percentage"
					}
				},
				"required": ["source_file"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleTestGenerator(argsJSON)
			},
		},
		{
			"project_analyzer",
			"Analyze entire Go projects for structure, dependencies, and metrics",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"project_path": {
						"type": "string",
						"description": "Path to the project root directory"
					},
					"include_vendor": {
						"type": "boolean",
						"default": false,
						"description": "Include vendor directory in analysis"
					},
					"output_format": {
						"type": "string",
						"enum": ["summary", "detailed", "json"],
						"default": "summary",
						"description": "Format of the analysis output"
					}
				},
				"required": ["project_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleProjectAnalyzer(argsJSON)
			},
		},
		{
			"refactor_assistant",
			"Suggest and apply code refactoring improvements",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {
						"type": "string",
						"description": "Path to the file to refactor"
					},
					"refactor_type": {
						"type": "string",
						"enum": ["extract_function", "rename", "move_method", "simplify", "optimize"],
						"description": "Type of refactoring to perform"
					},
					"target": {
						"type": "string",
						"description": "Specific target (function name, variable name, etc.)"
					},
					"dry_run": {
						"type": "boolean",
						"default": true,
						"description": "Only suggest changes without applying them"
					}
				},
				"required": ["file_path", "refactor_type"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleRefactorAssistant(argsJSON)
			},
		},
		{
			"api_tester",
			"Test REST APIs and generate documentation",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "API endpoint URL to test"
					},
					"method": {
						"type": "string",
						"enum": ["GET", "POST", "PUT", "DELETE", "PATCH"],
						"default": "GET",
						"description": "HTTP method to use"
					},
					"headers": {
						"type": "object",
						"description": "HTTP headers to include"
					},
					"body": {
						"type": "string",
						"description": "Request body (JSON string)"
					},
					"auth_type": {
						"type": "string",
						"enum": ["none", "bearer", "basic", "api_key"],
						"default": "none",
						"description": "Authentication type"
					},
					"auth_value": {
						"type": "string",
						"description": "Authentication value (token, key, etc.)"
					}
				},
				"required": ["url"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleAPITester(argsJSON)
			},
		},
		{
			"doc_generator",
			"Generate comprehensive documentation for Go packages and projects",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"package_path": {
						"type": "string",
						"description": "Path to the Go package to document"
					},
					"output_format": {
						"type": "string",
						"enum": ["markdown", "html", "godoc"],
						"default": "markdown",
						"description": "Output format for documentation"
					},
					"include_private": {
						"type": "boolean",
						"default": false,
						"description": "Include private/unexported items"
					},
					"include_examples": {
						"type": "boolean",
						"default": true,
						"description": "Generate usage examples"
					}
				},
				"required": ["package_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleDocGenerator(argsJSON)
			},
		},
		{
			"database_query",
			"Execute safe database queries and analyze schemas",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"connection_string": {
						"type": "string",
						"description": "Database connection string"
					},
					"query": {
						"type": "string",
						"description": "SQL query to execute (SELECT only for safety)"
					},
					"database_type": {
						"type": "string",
						"enum": ["sqlite", "postgres", "mysql"],
						"default": "sqlite",
						"description": "Type of database"
					},
					"limit": {
						"type": "integer",
						"default": 100,
						"maximum": 1000,
						"description": "Maximum number of rows to return"
					}
				},
				"required": ["connection_string", "query"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleDatabaseQuery(argsJSON)
			},
		},
		{
			"performance_profiler",
			"Profile Go applications and analyze performance bottlenecks",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"binary_path": {
						"type": "string",
						"description": "Path to the compiled Go binary"
					},
					"profile_type": {
						"type": "string",
						"enum": ["cpu", "memory", "goroutine", "block", "mutex"],
						"default": "cpu",
						"description": "Type of profiling to perform"
					},
					"duration": {
						"type": "integer",
						"default": 30,
						"minimum": 10,
						"maximum": 300,
						"description": "Profiling duration in seconds"
					},
					"args": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Command line arguments for the binary"
					}
				},
				"required": ["binary_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handlePerformanceProfiler(argsJSON)
			},
		},
		{
			"code_formatter",
			"Format and style Go code according to best practices",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {
						"type": "string",
						"description": "Path to the Go file to format"
					},
					"style": {
						"type": "string",
						"enum": ["gofmt", "goimports", "gofumpt"],
						"default": "gofmt",
						"description": "Formatting style to apply"
					},
					"fix_imports": {
						"type": "boolean",
						"default": true,
						"description": "Automatically fix import statements"
					},
					"dry_run": {
						"type": "boolean",
						"default": false,
						"description": "Show changes without applying them"
					}
				},
				"required": ["file_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleCodeFormatter(argsJSON)
			},
		},
		{
			"dependency_analyzer",
			"Analyze Go module dependencies and suggest optimizations",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"module_path": {
						"type": "string",
						"description": "Path to the Go module (directory with go.mod)"
					},
					"analysis_depth": {
						"type": "string",
						"enum": ["direct", "all", "outdated"],
						"default": "all",
						"description": "Depth of dependency analysis"
					},
					"check_vulnerabilities": {
						"type": "boolean",
						"default": true,
						"description": "Check for known security vulnerabilities"
					},
					"suggest_updates": {
						"type": "boolean",
						"default": true,
						"description": "Suggest dependency updates"
					}
				},
				"required": ["module_path"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleDependencyAnalyzer(argsJSON)
			},
		},
		{
			"git_assistant",
			"Intelligent Git operations and repository analysis",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo_path": {
						"type": "string",
						"description": "Path to the Git repository"
					},
					"operation": {
						"type": "string",
						"enum": ["status", "log", "diff", "blame", "contributors", "hotspots"],
						"description": "Git operation to perform"
					},
					"branch": {
						"type": "string",
						"description": "Branch name (for relevant operations)"
					},
					"file_path": {
						"type": "string",
						"description": "Specific file path (for file-specific operations)"
					},
					"limit": {
						"type": "integer",
						"default": 20,
						"description": "Limit for log entries or results"
					}
				},
				"required": ["repo_path", "operation"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleGitAssistant(argsJSON)
			},
		},
		{
			"ai_model_comparison",
			"Compare AI model responses and analyze performance",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"prompt": {
						"type": "string",
						"description": "Test prompt to send to models"
					},
					"models": {
						"type": "array",
						"items": {"type": "string"},
						"description": "List of model names to compare"
					},
					"evaluation_criteria": {
						"type": "array",
						"items": {
							"type": "string",
							"enum": ["accuracy", "creativity", "relevance", "coherence", "safety"]
						},
						"default": ["accuracy", "relevance"],
						"description": "Criteria for evaluation"
					},
					"temperature": {
						"type": "number",
						"default": 0.7,
						"minimum": 0,
						"maximum": 2,
						"description": "Temperature for model generation"
					}
				},
				"required": ["prompt", "models"]
			}`),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				argsJSON, _ := json.Marshal(args)
				return r.handleAIModelComparison(argsJSON)
			},
		},
	}

	for _, tool := range tools {
		// Convert the handler to match the tool manager's expected signature
		wrappedHandler := func(args json.RawMessage) (any, error) {
			var argMap map[string]interface{}
			if err := json.Unmarshal(args, &argMap); err != nil {
				return nil, err
			}
			return tool.handler(context.Background(), argMap)
		}
		
		if err := r.toolManager.RegisterTool(tool.name, tool.description, tool.parameters, wrappedHandler); err != nil {
			return fmt.Errorf("failed to register tool %s: %w", tool.name, err)
		}
	}

	return nil
}

// Tool handler implementations

func (r *AdvancedToolsRegistry) handleCodeAnalyzer(args json.RawMessage) (any, error) {
	var params struct {
		FilePath     string `json:"file_path"`
		AnalysisType string `json:"analysis_type"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Parse the Go file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, params.FilePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	analysis := make(map[string]interface{})

	if params.AnalysisType == "complexity" || params.AnalysisType == "all" {
		complexity := r.analyzeComplexity(node)
		analysis["complexity"] = complexity
	}

	if params.AnalysisType == "dependencies" || params.AnalysisType == "all" {
		deps := r.analyzeDependencies(node)
		analysis["dependencies"] = deps
	}

	if params.AnalysisType == "style" || params.AnalysisType == "all" {
		style := r.analyzeStyle(node, fset)
		analysis["style"] = style
	}

	if params.AnalysisType == "security" || params.AnalysisType == "all" {
		security := r.analyzeSecurityIssues(node)
		analysis["security"] = security
	}

	return map[string]interface{}{
		"file":     params.FilePath,
		"analysis": analysis,
		"timestamp": time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) analyzeComplexity(node *ast.File) map[string]interface{} {
	complexity := make(map[string]int)
	totalComplexity := 0

	ast.Inspect(node, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Name != nil {
				fnComplexity := r.calculateCyclomaticComplexity(fn)
				complexity[fn.Name.Name] = fnComplexity
				totalComplexity += fnComplexity
			}
		}
		return true
	})

	return map[string]interface{}{
		"functions":        complexity,
		"total_complexity": totalComplexity,
		"average_complexity": func() float64 {
			if len(complexity) == 0 {
				return 0
			}
			return float64(totalComplexity) / float64(len(complexity))
		}(),
	}
}

func (r *AdvancedToolsRegistry) calculateCyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1 // Base complexity

	ast.Inspect(fn, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		}
		return true
	})

	return complexity
}

func (r *AdvancedToolsRegistry) analyzeDependencies(node *ast.File) map[string]interface{} {
	imports := make(map[string]string)
	
	for _, imp := range node.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := path
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}

	return map[string]interface{}{
		"imports":      imports,
		"import_count": len(imports),
		"std_library":  r.countStandardLibraryImports(imports),
		"third_party":  r.countThirdPartyImports(imports),
	}
}

func (r *AdvancedToolsRegistry) countStandardLibraryImports(imports map[string]string) int {
	count := 0
	stdLibs := []string{
		"fmt", "os", "io", "net", "http", "time", "strings", "strconv", 
		"context", "sync", "encoding", "crypto", "database", "testing",
	}
	
	for _, path := range imports {
		for _, std := range stdLibs {
			if strings.HasPrefix(path, std) {
				count++
				break
			}
		}
	}
	return count
}

func (r *AdvancedToolsRegistry) countThirdPartyImports(imports map[string]string) int {
	count := 0
	for _, path := range imports {
		if strings.Contains(path, ".") && !strings.HasPrefix(path, "golang.org/x/") {
			count++
		}
	}
	return count
}

func (r *AdvancedToolsRegistry) analyzeStyle(node *ast.File, fset *token.FileSet) map[string]interface{} {
	issues := []string{}
	
	// Check for naming conventions
	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			if decl.Name != nil && decl.Name.IsExported() {
				if !isCapitalized(decl.Name.Name) {
					issues = append(issues, fmt.Sprintf("Exported function %s should start with capital letter", decl.Name.Name))
				}
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					if !isCapitalized(ts.Name.Name) {
						issues = append(issues, fmt.Sprintf("Exported type %s should start with capital letter", ts.Name.Name))
					}
				}
			}
		}
		return true
	})

	return map[string]interface{}{
		"issues":      issues,
		"issue_count": len(issues),
	}
}

func (r *AdvancedToolsRegistry) analyzeSecurityIssues(node *ast.File) map[string]interface{} {
	issues := []string{}
	
	// Look for potential security issues
	ast.Inspect(node, func(n ast.Node) bool {
		switch call := n.(type) {
		case *ast.CallExpr:
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				funcName := sel.Sel.Name
				// Check for dangerous functions
				if containsString([]string{"exec", "system", "eval"}, funcName) {
					issues = append(issues, fmt.Sprintf("Potentially dangerous function call: %s", funcName))
				}
			}
		}
		return true
	})

	return map[string]interface{}{
		"issues":      issues,
		"issue_count": len(issues),
		"severity":    "medium", // Could be enhanced with more sophisticated analysis
	}
}

func (r *AdvancedToolsRegistry) handleTestGenerator(args json.RawMessage) (any, error) {
	var params struct {
		SourceFile     string   `json:"source_file"`
		FunctionName   string   `json:"function_name"`
		TestTypes      []string `json:"test_types"`
		CoverageTarget float64  `json:"coverage_target"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Parse the source file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, params.SourceFile, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source file: %w", err)
	}

	// Generate test file path
	testFile := strings.TrimSuffix(params.SourceFile, ".go") + "_test.go"
	
	// Generate tests
	testCode := r.generateTestCode(node, params.FunctionName, params.TestTypes)

	return map[string]interface{}{
		"source_file":      params.SourceFile,
		"test_file":        testFile,
		"generated_code":   testCode,
		"coverage_target":  params.CoverageTarget,
		"test_types":       params.TestTypes,
		"functions_tested": r.extractFunctionNames(node, params.FunctionName),
	}, nil
}

func (r *AdvancedToolsRegistry) generateTestCode(node *ast.File, functionName string, testTypes []string) string {
	var buf strings.Builder
	
	buf.WriteString(fmt.Sprintf("package %s\n\n", node.Name.Name))
	buf.WriteString("import (\n\t\"testing\"\n)\n\n")

	// Extract functions to test
	functions := r.extractFunctionNames(node, functionName)
	
	for _, fn := range functions {
		for _, testType := range testTypes {
			switch testType {
			case "unit":
				buf.WriteString(r.generateUnitTest(fn))
			case "benchmark":
				buf.WriteString(r.generateBenchmarkTest(fn))
			case "fuzz":
				buf.WriteString(r.generateFuzzTest(fn))
			}
		}
	}

	return buf.String()
}

func (r *AdvancedToolsRegistry) generateUnitTest(functionName string) string {
	return fmt.Sprintf(`func Test%s(t *testing.T) {
	tests := []struct {
		name string
		// Add test parameters here
		want interface{}
	}{
		{
			name: "basic test",
			// Add test data here
			want: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call %s and assert results
			// got := %s(...)
			// if got != tt.want {
			//     t.Errorf("%s() = %%v, want %%v", got, tt.want)
			// }
		})
	}
}

`, functionName, functionName, functionName, functionName)
}

func (r *AdvancedToolsRegistry) generateBenchmarkTest(functionName string) string {
	return fmt.Sprintf(`func Benchmark%s(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Call %s here
		// %s(...)
	}
}

`, functionName, functionName, functionName)
}

func (r *AdvancedToolsRegistry) generateFuzzTest(functionName string) string {
	return fmt.Sprintf(`func Fuzz%s(f *testing.F) {
	f.Add("test input")
	f.Fuzz(func(t *testing.T, input string) {
		// Call %s with fuzz input
		// result := %s(input)
		// Add assertions here
	})
}

`, functionName, functionName, functionName)
}

func (r *AdvancedToolsRegistry) extractFunctionNames(node *ast.File, specific string) []string {
	var functions []string
	
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name != nil {
			if specific == "" || fn.Name.Name == specific {
				// Only include exported functions or the specific function
				if fn.Name.IsExported() || specific != "" {
					functions = append(functions, fn.Name.Name)
				}
			}
		}
		return true
	})
	
	return functions
}

func (r *AdvancedToolsRegistry) handleProjectAnalyzer(args json.RawMessage) (any, error) {
	var params struct {
		ProjectPath   string `json:"project_path"`
		IncludeVendor bool   `json:"include_vendor"`
		OutputFormat  string `json:"output_format"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	analysis := make(map[string]interface{})

	// Analyze project structure
	structure, err := r.analyzeProjectStructure(params.ProjectPath, params.IncludeVendor)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project structure: %w", err)
	}
	analysis["structure"] = structure

	// Analyze go.mod if it exists
	goModPath := filepath.Join(params.ProjectPath, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		modInfo, err := r.analyzeGoMod(goModPath)
		if err == nil {
			analysis["module"] = modInfo
		}
	}

	// Calculate metrics
	metrics, err := r.calculateProjectMetrics(params.ProjectPath)
	if err == nil {
		analysis["metrics"] = metrics
	}

	return map[string]interface{}{
		"project_path": params.ProjectPath,
		"analysis":     analysis,
		"timestamp":    time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) analyzeProjectStructure(projectPath string, includeVendor bool) (map[string]interface{}, error) {
	structure := make(map[string]interface{})
	fileCount := 0
	dirCount := 0
	goFiles := 0
	testFiles := 0

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(projectPath, path)
		
		// Skip vendor directory if not included
		if !includeVendor && strings.Contains(relPath, "vendor") {
			return nil
		}

		if info.IsDir() {
			dirCount++
		} else {
			fileCount++
			if strings.HasSuffix(path, ".go") {
				goFiles++
				if strings.HasSuffix(path, "_test.go") {
					testFiles++
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	structure["total_files"] = fileCount
	structure["total_directories"] = dirCount
	structure["go_files"] = goFiles
	structure["test_files"] = testFiles
	structure["test_coverage"] = float64(testFiles) / float64(goFiles-testFiles) * 100

	return structure, nil
}

func (r *AdvancedToolsRegistry) analyzeGoMod(goModPath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	modInfo := make(map[string]interface{})
	dependencies := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modInfo["module_name"] = strings.TrimPrefix(line, "module ")
		} else if strings.HasPrefix(line, "go ") {
			modInfo["go_version"] = strings.TrimPrefix(line, "go ")
		} else if strings.Contains(line, " v") && !strings.HasPrefix(line, "//") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dependencies[parts[0]] = parts[1]
			}
		}
	}

	modInfo["dependencies"] = dependencies
	modInfo["dependency_count"] = len(dependencies)

	return modInfo, nil
}

func (r *AdvancedToolsRegistry) calculateProjectMetrics(projectPath string) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})
	totalLines := 0
	totalComplexity := 0
	
	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			lines, complexity, err := r.analyzeFile(path)
			if err == nil {
				totalLines += lines
				totalComplexity += complexity
			}
		}

		return nil
	})

	metrics["total_lines"] = totalLines
	metrics["total_complexity"] = totalComplexity
	metrics["average_complexity_per_line"] = float64(totalComplexity) / float64(totalLines)

	return metrics, err
}

func (r *AdvancedToolsRegistry) analyzeFile(filePath string) (lines, complexity int, err error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, err
	}

	lines = strings.Count(string(content), "\n")

	// Parse for complexity analysis
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return lines, 0, nil // Return lines even if parsing fails
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt:
			complexity++
		}
		return true
	})

	return lines, complexity, nil
}

func (r *AdvancedToolsRegistry) handleRefactorAssistant(args json.RawMessage) (any, error) {
	var params struct {
		FilePath     string `json:"file_path"`
		RefactorType string `json:"refactor_type"`
		Target       string `json:"target"`
		DryRun       bool   `json:"dry_run"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Read the source file
	content, err := os.ReadFile(params.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the Go file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, params.FilePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	suggestions := []string{}
	newCode := string(content)

	switch params.RefactorType {
	case "extract_function":
		suggestions = r.suggestFunctionExtraction(node, params.Target)
	case "rename":
		suggestions = r.suggestRename(node, params.Target)
	case "simplify":
		suggestions = r.suggestSimplifications(node)
	case "optimize":
		suggestions = r.suggestOptimizations(node)
	default:
		return nil, fmt.Errorf("unsupported refactor type: %s", params.RefactorType)
	}

	result := map[string]interface{}{
		"file_path":     params.FilePath,
		"refactor_type": params.RefactorType,
		"suggestions":   suggestions,
		"dry_run":       params.DryRun,
	}

	if !params.DryRun && len(suggestions) > 0 {
		// Apply the first suggestion (in a real implementation, you'd want more sophisticated logic)
		result["applied"] = suggestions[0]
		result["new_code"] = newCode
	}

	return result, nil
}

func (r *AdvancedToolsRegistry) suggestFunctionExtraction(node *ast.File, target string) []string {
	suggestions := []string{}
	
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name != nil {
			complexity := r.calculateCyclomaticComplexity(fn)
			if complexity > 10 {
				suggestions = append(suggestions, 
					fmt.Sprintf("Function %s has high complexity (%d). Consider extracting smaller functions.", 
						fn.Name.Name, complexity))
			}
		}
		return true
	})

	return suggestions
}

func (r *AdvancedToolsRegistry) suggestRename(node *ast.File, target string) []string {
	suggestions := []string{}
	
	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			if decl.Name != nil && decl.Name.Name == target {
				if !isGoodName(decl.Name.Name) {
					suggestions = append(suggestions, 
						fmt.Sprintf("Consider renaming function %s to be more descriptive", target))
				}
			}
		}
		return true
	})

	return suggestions
}

func (r *AdvancedToolsRegistry) suggestSimplifications(node *ast.File) []string {
	suggestions := []string{}
	
	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.IfStmt:
			// Look for if statements that can be simplified
			if r.canSimplifyIf(stmt) {
				suggestions = append(suggestions, "Found if statement that can be simplified")
			}
		}
		return true
	})

	return suggestions
}

func (r *AdvancedToolsRegistry) suggestOptimizations(node *ast.File) []string {
	suggestions := []string{}
	
	ast.Inspect(node, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.CallExpr:
			// Look for string concatenation in loops
			if r.isStringConcatInLoop(expr) {
				suggestions = append(suggestions, "Consider using strings.Builder for string concatenation in loops")
			}
		}
		return true
	})

	return suggestions
}

func (r *AdvancedToolsRegistry) handleAPITester(args json.RawMessage) (any, error) {
	var params struct {
		URL       string                 `json:"url"`
		Method    string                 `json:"method"`
		Headers   map[string]string      `json:"headers"`
		Body      string                 `json:"body"`
		AuthType  string                 `json:"auth_type"`
		AuthValue string                 `json:"auth_value"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	var req *http.Request
	var err error

	if params.Body != "" {
		req, err = http.NewRequest(params.Method, params.URL, strings.NewReader(params.Body))
	} else {
		req, err = http.NewRequest(params.Method, params.URL, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range params.Headers {
		req.Header.Set(key, value)
	}

	// Add authentication
	switch params.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+params.AuthValue)
	case "basic":
		req.Header.Set("Authorization", "Basic "+params.AuthValue)
	case "api_key":
		req.Header.Set("X-API-Key", params.AuthValue)
	}

	// Make request
	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Collect response headers
	respHeaders := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			respHeaders[key] = values[0]
		}
	}

	return map[string]interface{}{
		"url":              params.URL,
		"method":           params.Method,
		"status_code":      resp.StatusCode,
		"status":           resp.Status,
		"headers":          respHeaders,
		"body":             string(body),
		"response_time_ms": duration.Milliseconds(),
		"content_length":   len(body),
		"timestamp":        time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) handleDocGenerator(args json.RawMessage) (any, error) {
	var params struct {
		PackagePath     string `json:"package_path"`
		OutputFormat    string `json:"output_format"`
		IncludePrivate  bool   `json:"include_private"`
		IncludeExamples bool   `json:"include_examples"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Parse the package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, params.PackagePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package: %w", err)
	}

	var doc strings.Builder
	packageName := ""

	for name, pkg := range pkgs {
		packageName = name
		doc.WriteString(r.generatePackageDoc(pkg, params.OutputFormat, params.IncludePrivate))
	}

	return map[string]interface{}{
		"package_path":     params.PackagePath,
		"package_name":     packageName,
		"output_format":    params.OutputFormat,
		"documentation":    doc.String(),
		"include_private":  params.IncludePrivate,
		"include_examples": params.IncludeExamples,
		"timestamp":        time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) generatePackageDoc(pkg *ast.Package, format string, includePrivate bool) string {
	var doc strings.Builder

	switch format {
	case "markdown":
		doc.WriteString(fmt.Sprintf("# Package %s\n\n", pkg.Name))
	case "html":
		doc.WriteString(fmt.Sprintf("<h1>Package %s</h1>\n", pkg.Name))
	}

	// Document each file in the package
	for _, file := range pkg.Files {
		r.documentFile(file, &doc, format, includePrivate)
	}

	return doc.String()
}

func (r *AdvancedToolsRegistry) documentFile(file *ast.File, doc *strings.Builder, format string, includePrivate bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			if decl.Name != nil && (includePrivate || decl.Name.IsExported()) {
				r.documentFunction(decl, doc, format)
			}
		case *ast.GenDecl:
			r.documentGenDecl(decl, doc, format, includePrivate)
		}
		return true
	})
}

func (r *AdvancedToolsRegistry) documentFunction(fn *ast.FuncDecl, doc *strings.Builder, format string) {
	switch format {
	case "markdown":
		doc.WriteString(fmt.Sprintf("## %s\n\n", fn.Name.Name))
		if fn.Doc != nil {
			doc.WriteString(fn.Doc.Text())
			doc.WriteString("\n")
		}
	case "html":
		doc.WriteString(fmt.Sprintf("<h2>%s</h2>\n", fn.Name.Name))
		if fn.Doc != nil {
			doc.WriteString(fmt.Sprintf("<p>%s</p>\n", fn.Doc.Text()))
		}
	}
}

func (r *AdvancedToolsRegistry) documentGenDecl(decl *ast.GenDecl, doc *strings.Builder, format string, includePrivate bool) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if includePrivate || s.Name.IsExported() {
				switch format {
				case "markdown":
					doc.WriteString(fmt.Sprintf("### Type %s\n\n", s.Name.Name))
				case "html":
					doc.WriteString(fmt.Sprintf("<h3>Type %s</h3>\n", s.Name.Name))
				}
			}
		}
	}
}

func (r *AdvancedToolsRegistry) handleDatabaseQuery(args json.RawMessage) (any, error) {
	var params struct {
		ConnectionString string `json:"connection_string"`
		Query            string `json:"query"`
		DatabaseType     string `json:"database_type"`
		Limit            int    `json:"limit"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Safety check: only allow SELECT queries
	queryUpper := strings.ToUpper(strings.TrimSpace(params.Query))
	if !strings.HasPrefix(queryUpper, "SELECT") {
		return nil, fmt.Errorf("only SELECT queries are allowed for safety")
	}

	// Connect to database
	var db *sql.DB
	var err error

	switch params.DatabaseType {
	case "sqlite":
		db, err = sql.Open("sqlite3", params.ConnectionString)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", params.DatabaseType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Add LIMIT to query if not present and limit is specified
	if params.Limit > 0 && !strings.Contains(queryUpper, "LIMIT") {
		params.Query = fmt.Sprintf("%s LIMIT %d", params.Query, params.Limit)
	}

	// Execute query
	rows, err := db.Query(params.Query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Read results
	var results []map[string]interface{}
	for rows.Next() {
		// Create a slice to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the result into the value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return map[string]interface{}{
		"query":       params.Query,
		"database":    params.DatabaseType,
		"columns":     columns,
		"results":     results,
		"row_count":   len(results),
		"timestamp":   time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) handlePerformanceProfiler(args json.RawMessage) (any, error) {
	var params struct {
		BinaryPath  string   `json:"binary_path"`
		ProfileType string   `json:"profile_type"`
		Duration    int      `json:"duration"`
		Args        []string `json:"args"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Check if binary exists
	if _, err := os.Stat(params.BinaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found: %s", params.BinaryPath)
	}

	// Create profile output file
	profileFile := fmt.Sprintf("/tmp/profile_%s_%d.prof", params.ProfileType, time.Now().Unix())

	// Build command arguments
	cmdArgs := append([]string{params.BinaryPath}, params.Args...)
	cmdArgs = append(cmdArgs, fmt.Sprintf("-cpuprofile=%s", profileFile))

	// Execute the binary with profiling
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(params.Duration)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.CombinedOutput()

	result := map[string]interface{}{
		"binary_path":    params.BinaryPath,
		"profile_type":   params.ProfileType,
		"duration":       params.Duration,
		"profile_file":   profileFile,
		"command_output": string(output),
		"timestamp":      time.Now().UTC(),
	}

	if err != nil {
		result["error"] = err.Error()
		return result, nil // Don't fail completely, return partial results
	}

	// Analyze the profile if it was created
	if _, err := os.Stat(profileFile); err == nil {
		analysis := r.analyzeProfile(profileFile, params.ProfileType)
		result["analysis"] = analysis
	}

	return result, nil
}

func (r *AdvancedToolsRegistry) analyzeProfile(profileFile, profileType string) map[string]interface{} {
	// This is a simplified analysis - in practice you'd use go tool pprof
	analysis := map[string]interface{}{
		"profile_file": profileFile,
		"type":         profileType,
	}

	// Try to get basic file info
	if info, err := os.Stat(profileFile); err == nil {
		analysis["file_size"] = info.Size()
		analysis["created"] = info.ModTime()
	}

	return analysis
}

func (r *AdvancedToolsRegistry) handleCodeFormatter(args json.RawMessage) (any, error) {
	var params struct {
		FilePath   string `json:"file_path"`
		Style      string `json:"style"`
		FixImports bool   `json:"fix_imports"`
		DryRun     bool   `json:"dry_run"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Read the source file
	content, err := os.ReadFile(params.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, params.FilePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	// Format the code
	var formatted strings.Builder
	switch params.Style {
	case "gofmt":
		err = format.Node(&formatted, fset, node)
	case "goimports":
		// Simulate goimports behavior
		err = format.Node(&formatted, fset, node)
	case "gofumpt":
		// Simulate gofumpt behavior
		err = format.Node(&formatted, fset, node)
	default:
		err = format.Node(&formatted, fset, node)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to format code: %w", err)
	}

	result := map[string]interface{}{
		"file_path":     params.FilePath,
		"style":         params.Style,
		"fix_imports":   params.FixImports,
		"dry_run":       params.DryRun,
		"original_size": len(content),
		"formatted_size": formatted.Len(),
		"changed":       string(content) != formatted.String(),
	}

	if params.DryRun {
		result["formatted_code"] = formatted.String()
	} else if string(content) != formatted.String() {
		// Write the formatted code back to the file
		if err := os.WriteFile(params.FilePath, []byte(formatted.String()), 0644); err != nil {
			return nil, fmt.Errorf("failed to write formatted code: %w", err)
		}
		result["applied"] = true
	}

	return result, nil
}

func (r *AdvancedToolsRegistry) handleDependencyAnalyzer(args json.RawMessage) (any, error) {
	var params struct {
		ModulePath           string `json:"module_path"`
		AnalysisDepth        string `json:"analysis_depth"`
		CheckVulnerabilities bool   `json:"check_vulnerabilities"`
		SuggestUpdates       bool   `json:"suggest_updates"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Check if go.mod exists
	goModPath := filepath.Join(params.ModulePath, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("go.mod not found in %s", params.ModulePath)
	}

	result := map[string]interface{}{
		"module_path":             params.ModulePath,
		"analysis_depth":          params.AnalysisDepth,
		"check_vulnerabilities":   params.CheckVulnerabilities,
		"suggest_updates":         params.SuggestUpdates,
		"timestamp":               time.Now().UTC(),
	}

	// Analyze go.mod
	modInfo, err := r.analyzeGoMod(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze go.mod: %w", err)
	}
	result["module_info"] = modInfo

	// Run go list to get dependency information
	if deps, err := r.getDependencyInfo(params.ModulePath, params.AnalysisDepth); err == nil {
		result["dependencies"] = deps
	}

	// Check for outdated dependencies if requested
	if params.SuggestUpdates {
		if updates, err := r.checkForUpdates(params.ModulePath); err == nil {
			result["available_updates"] = updates
		}
	}

	return result, nil
}

func (r *AdvancedToolsRegistry) getDependencyInfo(modulePath, depth string) (map[string]interface{}, error) {
	var cmd *exec.Cmd
	
	switch depth {
	case "direct":
		cmd = exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Version}}", "all")
	case "all":
		cmd = exec.Command("go", "list", "-m", "-json", "all")
	case "outdated":
		cmd = exec.Command("go", "list", "-u", "-m", "-json", "all")
	default:
		cmd = exec.Command("go", "list", "-m", "all")
	}

	cmd.Dir = modulePath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run go list: %w", err)
	}

	// Parse the output
	dependencies := map[string]interface{}{
		"raw_output": string(output),
		"count":      strings.Count(string(output), "\n"),
	}

	return dependencies, nil
}

func (r *AdvancedToolsRegistry) checkForUpdates(modulePath string) ([]string, error) {
	cmd := exec.Command("go", "list", "-u", "-m", "all")
	cmd.Dir = modulePath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var updates []string
	
	for _, line := range lines {
		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			updates = append(updates, line)
		}
	}

	return updates, nil
}

func (r *AdvancedToolsRegistry) handleGitAssistant(args json.RawMessage) (any, error) {
	var params struct {
		RepoPath  string `json:"repo_path"`
		Operation string `json:"operation"`
		Branch    string `json:"branch"`
		FilePath  string `json:"file_path"`
		Limit     int    `json:"limit"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Check if it's a git repository
	gitDir := filepath.Join(params.RepoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", params.RepoPath)
	}

	var cmd *exec.Cmd
	switch params.Operation {
	case "status":
		cmd = exec.Command("git", "status", "--porcelain")
	case "log":
		args := []string{"log", "--oneline"}
		if params.Limit > 0 {
			args = append(args, fmt.Sprintf("-%d", params.Limit))
		}
		cmd = exec.Command("git", args...)
	case "diff":
		if params.FilePath != "" {
			cmd = exec.Command("git", "diff", params.FilePath)
		} else {
			cmd = exec.Command("git", "diff")
		}
	case "blame":
		if params.FilePath == "" {
			return nil, fmt.Errorf("file_path required for blame operation")
		}
		cmd = exec.Command("git", "blame", params.FilePath)
	case "contributors":
		cmd = exec.Command("git", "shortlog", "-sn")
	case "hotspots":
		cmd = exec.Command("git", "log", "--format=format:", "--name-only")
	default:
		return nil, fmt.Errorf("unsupported operation: %s", params.Operation)
	}

	cmd.Dir = params.RepoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git command failed: %w", err)
	}

	result := map[string]interface{}{
		"repo_path":  params.RepoPath,
		"operation":  params.Operation,
		"output":     string(output),
		"timestamp":  time.Now().UTC(),
	}

	// Parse output for specific operations
	switch params.Operation {
	case "status":
		result["files"] = r.parseGitStatus(string(output))
	case "log":
		result["commits"] = r.parseGitLog(string(output))
	case "contributors":
		result["contributors"] = r.parseContributors(string(output))
	}

	return result, nil
}

func (r *AdvancedToolsRegistry) parseGitStatus(output string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var files []map[string]string

	for _, line := range lines {
		if len(line) >= 3 {
			files = append(files, map[string]string{
				"status": line[:2],
				"file":   line[3:],
			})
		}
	}

	return files
}

func (r *AdvancedToolsRegistry) parseGitLog(output string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var commits []map[string]string

	for _, line := range lines {
		if line != "" {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				commits = append(commits, map[string]string{
					"hash":    parts[0],
					"message": parts[1],
				})
			}
		}
	}

	return commits
}

func (r *AdvancedToolsRegistry) parseContributors(output string) []map[string]interface{} {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var contributors []map[string]interface{}

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := strings.Join(parts[1:], " ")
			contributors = append(contributors, map[string]interface{}{
				"commits": parts[0],
				"name":    name,
			})
		}
	}

	return contributors
}

func (r *AdvancedToolsRegistry) handleAIModelComparison(args json.RawMessage) (any, error) {
	var params struct {
		Prompt             string   `json:"prompt"`
		Models             []string `json:"models"`
		EvaluationCriteria []string `json:"evaluation_criteria"`
		Temperature        float64  `json:"temperature"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// This is a simplified implementation
	// In a real scenario, you'd integrate with actual AI model APIs
	results := make(map[string]interface{})
	
	for _, model := range params.Models {
		// Simulate model responses (in practice, call actual APIs)
		modelResult := map[string]interface{}{
			"model":       model,
			"prompt":      params.Prompt,
			"response":    fmt.Sprintf("Simulated response from %s", model),
			"temperature": params.Temperature,
			"timestamp":   time.Now().UTC(),
		}

		// Simulate evaluation scores
		scores := make(map[string]float64)
		for _, criterion := range params.EvaluationCriteria {
			// Generate random scores for simulation
			scores[criterion] = 0.7 + (0.3 * (float64(len(model)) / 20.0)) // Simplified scoring
		}
		modelResult["scores"] = scores

		results[model] = modelResult
	}

	// Calculate comparative analysis
	comparison := r.compareModelResults(results, params.EvaluationCriteria)
	
	return map[string]interface{}{
		"prompt":              params.Prompt,
		"models":              params.Models,
		"evaluation_criteria": params.EvaluationCriteria,
		"results":             results,
		"comparison":          comparison,
		"timestamp":           time.Now().UTC(),
	}, nil
}

func (r *AdvancedToolsRegistry) compareModelResults(results map[string]interface{}, criteria []string) map[string]interface{} {
	comparison := make(map[string]interface{})
	
	// Find best performing model for each criterion
	for _, criterion := range criteria {
		bestModel := ""
		bestScore := 0.0
		
		for model, result := range results {
			if modelResult, ok := result.(map[string]interface{}); ok {
				if scores, ok := modelResult["scores"].(map[string]float64); ok {
					if score, exists := scores[criterion]; exists && score > bestScore {
						bestScore = score
						bestModel = model
					}
				}
			}
		}
		
		comparison[criterion] = map[string]interface{}{
			"best_model": bestModel,
			"best_score": bestScore,
		}
	}
	
	return comparison
}

// Helper functions

func isCapitalized(name string) bool {
	if len(name) == 0 {
		return false
	}
	first := name[0]
	return first >= 'A' && first <= 'Z'
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isGoodName(name string) bool {
	// Simple heuristic for good naming
	return len(name) > 2 && !containsString([]string{"a", "b", "c", "x", "y", "z"}, name)
}

func (r *AdvancedToolsRegistry) canSimplifyIf(stmt *ast.IfStmt) bool {
	// Simple check for if statements that can be simplified
	// This is a basic implementation
	return stmt.Else == nil && stmt.Init == nil
}

func (r *AdvancedToolsRegistry) isStringConcatInLoop(expr *ast.CallExpr) bool {
	// Check if this is a string concatenation operation
	// This is a simplified check
	if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name == "concat" || sel.Sel.Name == "Join"
	}
	return false
}