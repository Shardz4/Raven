package validation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// Result holds the outcome of a safety validation check.
type Result struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// ForbiddenImports lists Python modules that are dangerous in untrusted code.
var ForbiddenImports = map[string]bool{
	"os": true, "subprocess": true, "socket": true, "ssl": true,
	"asyncio": true, "multiprocessing": true, "threading": true, "signal": true,
	"pathlib": true, "shutil": true, "glob": true, "tempfile": true,
	"importlib": true, "pickle": true, "marshal": true,
	"requests": true, "urllib": true, "http": true, "ftplib": true,
	"docker": true,
}

// ForbiddenCalls lists Python builtin calls that are dangerous.
var ForbiddenCalls = map[string]bool{
	"eval": true, "exec": true, "compile": true, "__import__": true, "open": true,
}

// ValidatePythonPatch performs lightweight safety checks on Python code.
// This is a regex-based heuristic (not a full Python AST parser in Go),
// but it catches the most common dangerous patterns before Docker sandbox.
func ValidatePythonPatch(code string) *Result {
	code = strings.TrimSpace(code)
	if code == "" {
		return &Result{OK: false, Reason: "empty patch"}
	}
	if len(code) > 20000 {
		return &Result{OK: false, Reason: "patch too large (>20000 chars)"}
	}

	// Check for forbidden imports
	importRe := regexp.MustCompile(`(?m)^\s*(?:import|from)\s+(\w+)`)
	for _, match := range importRe.FindAllStringSubmatch(code, -1) {
		if len(match) > 1 && ForbiddenImports[match[1]] {
			return &Result{OK: false, Reason: "forbidden import: " + match[1]}
		}
	}

	// Check for forbidden calls
	for call := range ForbiddenCalls {
		callRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(call) + `\s*\(`)
		if callRe.MatchString(code) {
			return &Result{OK: false, Reason: "forbidden call: " + call + "()"}
		}
	}

	return &Result{OK: true, Reason: "OK"}
}

// NormalizePythonCode strips trailing whitespace and collapses the code to a
// canonical form for structural comparison in the consensus engine.
func NormalizePythonCode(code string) string {
	lines := strings.Split(strings.TrimSpace(code), "\n")
	var normalized []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		// Skip blank lines and comments for structural comparison
		if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return strings.Join(normalized, "\n")
}

// StructuralFingerprint creates a rough structural fingerprint of Python code
// by extracting the "shape" — function names, class names, control flow keywords,
// and call patterns — while ignoring variable names and string literals.
// Two patches with the same fingerprint are "structurally equivalent".
func StructuralFingerprint(code string) string {
	// Extract structural elements using regex
	structural := []string{}

	// Function definitions
	funcRe := regexp.MustCompile(`(?m)^\s*def\s+(\w+)\s*\(`)
	for _, m := range funcRe.FindAllStringSubmatch(code, -1) {
		structural = append(structural, "DEF:"+m[1])
	}

	// Class definitions
	classRe := regexp.MustCompile(`(?m)^\s*class\s+(\w+)`)
	for _, m := range classRe.FindAllStringSubmatch(code, -1) {
		structural = append(structural, "CLASS:"+m[1])
	}

	// Control flow
	for _, kw := range []string{"if", "elif", "else", "for", "while", "try", "except", "finally", "with", "return", "raise", "yield"} {
		kwRe := regexp.MustCompile(`(?m)^\s*` + kw + `\b`)
		count := len(kwRe.FindAllString(code, -1))
		if count > 0 {
			structural = append(structural, strings.ToUpper(kw)+":"+strings.Repeat("x", count))
		}
	}

	return strings.Join(structural, "|")
}

// ValidateGoCode is a helper for validating Go code patches (bonus capability).
func ValidateGoCode(code string) *Result {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "patch.go", code, parser.AllErrors)
	if err != nil {
		return &Result{OK: false, Reason: "Go syntax error: " + err.Error()}
	}
	return &Result{OK: true, Reason: "OK"}
}

// Keep ast import used for Go validation
var _ = ast.NewIdent
