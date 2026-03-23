package providers

import "regexp"

// surrogateEscapeRe matches JSON-escaped lone surrogate code points (\uD800–\uDFFF).
// These are invalid in JSON per RFC 8259 and cause API rejections.
var surrogateEscapeRe = regexp.MustCompile(`\\u[dD][89a-fA-F][0-9a-fA-F]{2}`)

// sanitizeJSONSurrogates replaces lone surrogate escapes in JSON bytes with
// the Unicode replacement character (\uFFFD). This handles surrogates that
// leak through json.RawMessage fields which bypass Go's json.Marshal cleaning.
func sanitizeJSONSurrogates(data []byte) []byte {
	if !surrogateEscapeRe.Match(data) {
		return data
	}
	return surrogateEscapeRe.ReplaceAll(data, []byte(`\uFFFD`))
}
