package ardenn

import (
	"bytes"
	"text/template"
)

func ResolveTemplate(tmpl string, vars map[string]any) string {
	if tmpl == "" {
		return ""
	}
	t, err := template.New("task").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return tmpl
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return tmpl
	}
	return buf.String()
}

func MergeVariables(maps ...map[string]any) map[string]any {
	result := map[string]any{}
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
