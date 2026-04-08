package orchestrator

import (
	"fmt"
	"strconv"
	"strings"
)

// EvalExpr evaluates simple expressions used in When/AutoPass fields.
// Supports: "in [...]", "<=", ">=", "<", ">", "==", "!=", "&&"
// Variables resolved from map via "variables.X" or "eval.X" prefix.
func EvalExpr(expr string, vars map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Handle && (AND)
	if parts := splitAnd(expr); len(parts) > 1 {
		for _, p := range parts {
			result, err := EvalExpr(p, vars)
			if err != nil {
				return false, err
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	}

	// Handle "X in ['a','b','c']"
	if idx := strings.Index(expr, " in ["); idx > 0 {
		return evalIn(expr, idx, vars)
	}

	// Handle comparisons: <=, >=, ==, !=, <, >
	for _, op := range []string{"<=", ">=", "==", "!=", "<", ">"} {
		if idx := strings.Index(expr, " "+op+" "); idx > 0 {
			return evalCompare(expr[:idx], strings.TrimSpace(expr[idx+len(op)+2:]), op, vars)
		}
	}

	return false, fmt.Errorf("unsupported expression: %s", expr)
}

func resolveVar(name string, vars map[string]any) any {
	name = strings.TrimSpace(name)
	for _, prefix := range []string{"variables.", "eval."} {
		if strings.HasPrefix(name, prefix) {
			key := strings.TrimPrefix(name, prefix)
			if v, ok := vars[key]; ok {
				return v
			}
			if v, ok := vars[name]; ok {
				return v
			}
			return nil
		}
	}
	if v, ok := vars[name]; ok {
		return v
	}
	return nil
}

func evalIn(expr string, idx int, vars map[string]any) (bool, error) {
	varName := strings.TrimSpace(expr[:idx])
	listStr := strings.TrimSpace(expr[idx+4:]) // skip " in "
	listStr = strings.Trim(listStr, "[]")

	val := resolveVar(varName, vars)
	valStr := fmt.Sprintf("%v", val)

	items := strings.Split(listStr, ",")
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "'\"")
		if item == valStr {
			return true, nil
		}
	}
	return false, nil
}

func evalCompare(leftExpr, rightStr string, op string, vars map[string]any) (bool, error) {
	left := resolveVar(leftExpr, vars)
	rightStr = strings.TrimSpace(rightStr)

	leftNum := toFloat(left)
	rightNum, err := strconv.ParseFloat(rightStr, 64)
	if err != nil {
		leftStr := fmt.Sprintf("%v", left)
		switch op {
		case "==":
			return leftStr == rightStr, nil
		case "!=":
			return leftStr != rightStr, nil
		default:
			return false, fmt.Errorf("non-numeric comparison with %s", op)
		}
	}

	switch op {
	case "<=":
		return leftNum <= rightNum, nil
	case ">=":
		return leftNum >= rightNum, nil
	case "<":
		return leftNum < rightNum, nil
	case ">":
		return leftNum > rightNum, nil
	case "==":
		return leftNum == rightNum, nil
	case "!=":
		return leftNum != rightNum, nil
	}
	return false, nil
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func splitAnd(expr string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		}
		if depth == 0 && i+4 <= len(expr) && expr[i:i+4] == " && " {
			parts = append(parts, strings.TrimSpace(expr[start:i]))
			start = i + 4
		}
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}
