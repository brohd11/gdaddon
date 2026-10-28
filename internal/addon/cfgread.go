package addon

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

func loadConfig(path string) (*ini.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConfig(data)
}

// parseConfig protects Godot arrays from INI's line and inline-comment rules.
// Scalars still use the existing parser; the original bytes are never rewritten.
func parseConfig(data []byte) (*ini.File, error) {
	text := string(data)
	prefix := "__gdaddon_array_"
	for strings.Contains(text, prefix) {
		prefix += "_"
	}
	arrays := make(map[string]string)
	var out strings.Builder
	quote, continuation := "", false
	line := 1
	for offset := 0; offset < len(text); {
		end := strings.IndexByte(text[offset:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += offset + 1
		}
		raw := text[offset:end]
		trimmed := strings.TrimSpace(raw)
		switch {
		case quote != "":
			if i := strings.LastIndex(trimmed, quote); i >= 0 && !strings.HasSuffix(strings.TrimSpace(trimmed[i+len(quote):]), `\`) {
				quote = ""
			}
		case continuation:
			continuation = strings.HasSuffix(trimmed, `\`)
		case trimmed != "" && !strings.ContainsAny(trimmed[:1], "[;#"):
			if eq := configDelimiter(raw); eq >= 0 {
				value := strings.TrimLeft(raw[eq+1:], " \t")
				if strings.HasPrefix(value, "[") {
					start := offset + len(raw) - len(value)
					array, stop, err := configArray(text, start)
					if err != nil {
						return nil, fmt.Errorf("key %s at line %d: %w", strings.TrimSpace(raw[:eq]), line, err)
					}
					// Only whitespace or a comment may follow the closing bracket.
					end = stop
					for end < len(text) && text[end] != '\n' {
						end++
					}
					if suffix := strings.TrimSpace(text[stop:end]); suffix != "" && suffix[0] != ';' && suffix[0] != '#' {
						return nil, fmt.Errorf("key %s at line %d: unexpected text after array", strings.TrimSpace(raw[:eq]), line)
					}
					if end < len(text) {
						end++
					}
					token := fmt.Sprintf("%s%d__", prefix, len(arrays))
					arrays[token] = array
					out.WriteString(text[offset:start])
					out.WriteString(token)
					out.WriteString(text[stop:end])
					line += strings.Count(text[offset:end], "\n")
					offset = end
					continue
				}
				// INI supports triple quotes, backticks and backslash continuations.
				quoted := false
				for _, marker := range []string{`"""`, "`"} {
					if strings.HasPrefix(value, marker) {
						quoted = true
						if !strings.Contains(value[len(marker):], marker) {
							quote = marker
						}
						break
					}
				}
				continuation = !quoted && strings.HasSuffix(trimmed, `\`)
			}
		}
		out.WriteString(raw)
		line += strings.Count(raw, "\n")
		offset = end
	}
	cfg, err := ini.Load([]byte(out.String()))
	if err != nil {
		return nil, err
	}
	for _, section := range cfg.Sections() {
		for _, key := range section.Keys() {
			if value, ok := arrays[key.Value()]; ok {
				key.SetValue(value)
			}
		}
	}
	return cfg, nil
}

// configDelimiter skips quoted key names, which may themselves contain '=' or ':'.
func configDelimiter(line string) int {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		} else if c == '"' || c == '\'' || c == '`' {
			quote = c
		} else if c == '=' || c == ':' {
			return i
		}
	}
	return -1
}

// configArray retains literal content, removing only comments outside strings.
func configArray(text string, start int) (string, int, error) {
	var out strings.Builder
	depth := 0
	var quote byte
	for i := start; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(text) {
				i++
				out.WriteByte(text[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case ';', '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
			continue
		case '[':
			depth++
		case ']':
			depth--
		}
		out.WriteByte(c)
		if depth == 0 {
			return out.String(), i + 1, nil
		}
	}
	return "", 0, fmt.Errorf("unterminated array")
}
