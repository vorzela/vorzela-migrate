package generate

import (
	"strings"
	"unicode"
)

// initialisms are rendered fully upper-case in generated identifiers so the
// output reads like hand-written Go (UserID, not UserId).
var initialisms = map[string]string{
	"id": "ID", "ids": "IDs", "url": "URL", "uri": "URI", "uuid": "UUID",
	"api": "API", "http": "HTTP", "https": "HTTPS", "html": "HTML",
	"json": "JSON", "jsonb": "JSONB", "xml": "XML", "sql": "SQL", "db": "DB",
	"ip": "IP", "tcp": "TCP", "udp": "UDP", "cpu": "CPU", "ttl": "TTL",
	"acl": "ACL", "cdn": "CDN", "dns": "DNS", "sso": "SSO", "otp": "OTP",
	"pdf": "PDF", "csv": "CSV", "utc": "UTC", "eof": "EOF", "os": "OS",
	"vat": "VAT", "sku": "SKU", "isbn": "ISBN", "mime": "MIME", "gpu": "GPU",
}

// GoName converts a snake_case database identifier to an exported Go name.
func GoName(s string) string {
	parts := splitIdent(s)
	var b strings.Builder
	for _, p := range parts {
		if up, ok := initialisms[strings.ToLower(p)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(strings.ToLower(p[1:]))
		}
	}
	name := b.String()
	if name == "" {
		return "X"
	}
	if unicode.IsDigit(rune(name[0])) {
		return "X" + name
	}
	return name
}

// GoFieldName is GoName, kept separate so column-specific rules can diverge.
func GoFieldName(col string) string { return GoName(col) }

// EnumGoName is the Go type name for a database enum type.
func EnumGoName(enum string) string { return GoName(enum) }

// EnumValueConst builds the constant name for one enum value.
func EnumValueConst(enumGoName, value string) string {
	suffix := GoName(value)
	if suffix == "" || suffix == "X" {
		suffix = "Value" + sanitizeSuffix(value)
	}
	return enumGoName + suffix
}

func sanitizeSuffix(v string) string {
	var b strings.Builder
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Unknown"
	}
	return b.String()
}

func splitIdent(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' '
	})
	var out []string
	for _, f := range fields {
		out = append(out, splitCamel(f)...)
	}
	return out
}

// splitCamel breaks camelCase / PascalCase runs so mixed-style names normalize.
func splitCamel(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		boundary := unicode.IsLower(prev) && unicode.IsUpper(cur)
		if !boundary && unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			boundary = true
		}
		if !boundary && unicode.IsDigit(prev) != unicode.IsDigit(cur) {
			boundary = true
		}
		if boundary {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	return append(out, string(runes[start:]))
}

// Singular is a pragmatic English singularizer for table names.
func Singular(table string) string {
	lower := strings.ToLower(table)
	switch {
	case strings.HasSuffix(lower, "ies") && len(table) > 3:
		return table[:len(table)-3] + "y"
	case strings.HasSuffix(lower, "sses"), strings.HasSuffix(lower, "shes"),
		strings.HasSuffix(lower, "ches"), strings.HasSuffix(lower, "xes"),
		strings.HasSuffix(lower, "zes"):
		return table[:len(table)-2]
	case strings.HasSuffix(lower, "ses") && len(table) > 3:
		return table[:len(table)-2]
	case strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && len(table) > 1:
		return table[:len(table)-1]
	}
	return table
}

// Plural is a pragmatic English pluralizer for relation names.
func Plural(name string) string {
	lower := strings.ToLower(name)
	switch {
	case lower == "":
		return name
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"), strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "sh"):
		return name + "es"
	case strings.HasSuffix(lower, "y") && len(name) > 1 && !isVowel(rune(lower[len(lower)-2])):
		return name[:len(name)-1] + "ies"
	default:
		return name + "s"
	}
}

func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// ModelName is the struct name for a table.
func ModelName(table string) string { return GoName(Singular(table)) }

// EntityName is the package-level query handle name for a table.
func EntityName(table string) string { return GoName(table) }

// FileName is the generated file name for a table's model.
func FileName(table string) string {
	return strings.ToLower(Singular(strings.ReplaceAll(table, ".", "_"))) + "_gen.go"
}
