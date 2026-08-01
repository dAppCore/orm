// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"slices"

	"dappco.re/go"
)

// Shaper defines bidirectional shape enforcement — incoming values are
// coerced to the column's declared shape; stored values are rehydrated
// back to typed Go before reaching consumer structs.
type Shaper interface {
	Coerce(v any) core.Result
	Rehydrate(v any) core.Result
}

// --- Symmetric shapers (Rehydrate is identity) ---

type stringShaper struct{}

func (stringShaper) Coerce(v any) core.Result {
	if v == nil {
		return core.Fail(core.NewCode("orm.input.null", "null value for string field"))
	}
	s, ok := v.(string)
	if !ok {
		return core.Fail(core.NewCode("orm.input.type", "expected string"))
	}
	return core.Ok(s)
}

func (stringShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type emailShaper struct{}

var compiledEmailRe = core.Regex(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func init() {
	mustCompiledRegex(compiledEmailRe, "invalid email regex")
	mustCompiledRegex(compiledUUIDRe, "invalid UUID regex")
	mustCompiledRegex(compiledHostnameRe, "invalid hostname regex")
}

func mustCompiledRegex(r core.Result, message string) {
	if !r.OK {
		panic(message)
	}
}

func (emailShaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	if s == "" {
		return core.Fail(core.NewCode("orm.input.null", "empty email"))
	}
	// Extract email from angle-bracket notation like "Name <user@host.com>"
	if core.Contains(s, "<") && core.Contains(s, ">") {
		start := -1
		end := -1
		for i, c := range s {
			if c == '<' {
				start = i
			}
			if c == '>' {
				end = i
				break
			}
		}
		if start >= 0 && end > start {
			s = s[start+1 : end]
		}
	}
	s = trimSuffix(trimPrefix(s, "mailto:"), "/")
	if !compiledEmailRe.Value.(*core.Regexp).MatchString(s) {
		return core.Fail(core.NewCode("orm.input.format", "invalid email format"))
	}
	return core.Ok(s)
}

func (emailShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type urlShaper struct{}

func (urlShaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	if s == "" {
		return core.Fail(core.NewCode("orm.input.null", "empty URL"))
	}
	parsed := core.URLParse(s)
	if !parsed.OK {
		return core.Fail(core.NewCode("orm.input.format", "invalid URL"))
	}
	u := parsed.Value.(*core.URL)
	if u.Scheme == "" {
		return core.Fail(core.NewCode("orm.input.format", "URL must have a scheme (https://...)"))
	}
	return core.Ok(s)
}

func (urlShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type uuidShaper struct{}

var compiledUUIDRe = core.Regex(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (uuidShaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	if s == "" {
		return core.Fail(core.NewCode("orm.input.null", "empty UUID"))
	}
	if !compiledUUIDRe.Value.(*core.Regexp).MatchString(s) {
		return core.Fail(core.NewCode("orm.input.format", "invalid UUID format"))
	}
	return core.Ok(s)
}

func (uuidShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type ipv4Shaper struct{}

func (ipv4Shaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	ip := core.ParseIP(s)
	if ip == nil {
		return core.Fail(core.NewCode("orm.input.format", "invalid IPv4 address"))
	}
	if ip.To4() == nil {
		return core.Fail(core.NewCode("orm.input.format", "not an IPv4 address"))
	}
	return core.Ok(s)
}

func (ipv4Shaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type ipv6Shaper struct{}

func (ipv6Shaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	ip := core.ParseIP(s)
	if ip == nil {
		return core.Fail(core.NewCode("orm.input.format", "invalid IPv6 address"))
	}
	if ip.To4() != nil {
		return core.Fail(core.NewCode("orm.input.format", "not an IPv6 address"))
	}
	return core.Ok(s)
}

func (ipv6Shaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type hostnameShaper struct{}

var compiledHostnameRe = core.Regex(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)

func (hostnameShaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	if s == "" {
		return core.Fail(core.NewCode("orm.input.null", "empty hostname"))
	}
	if len(s) > 253 {
		return core.Fail(core.NewCode("orm.input.range", "hostname too long"))
	}
	if !compiledHostnameRe.Value.(*core.Regexp).MatchString(s) {
		return core.Fail(core.NewCode("orm.input.format", "invalid hostname"))
	}
	return core.Ok(s)
}

func (hostnameShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

type slugShaper struct{}

func (slugShaper) Coerce(v any) core.Result {
	r := stringShaper{}.Coerce(v)
	if !r.OK {
		return r
	}
	s := r.Value.(string)
	s = slugify(s)
	return core.Ok(s)
}

func (slugShaper) Rehydrate(v any) core.Result {
	return stringShaper{}.Coerce(v)
}

func trimPrefix(s, prefix string) string {
	if core.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if core.HasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func slugify(s string) string {
	lowered := core.Lower(s)
	var result []byte
	lastDash := false
	for i := range len(lowered) {
		c := lowered[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, c)
			lastDash = false
		} else if c == '-' || c == ' ' || c == '_' {
			if !lastDash && len(result) > 0 {
				result = append(result, '-')
				lastDash = true
			}
		}
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}

type intShaper struct{}

func (intShaper) Coerce(v any) core.Result {
	switch val := v.(type) {
	case int64:
		return core.Ok(val)
	case int:
		return core.Ok(int64(val))
	case int32:
		return core.Ok(int64(val))
	case float64:
		if val != float64(int64(val)) {
			return core.Fail(core.NewCode("orm.input.type", "expected integer"))
		}
		return core.Ok(int64(val))
	case string:
		parsedR := core.ParseInt(val, 10, 64)
		if !parsedR.OK {
			return core.Fail(core.NewCode("orm.input.type", "expected integer"))
		}
		return core.Ok(parsedR.Value.(int64))
	default:
		return core.Fail(core.NewCode("orm.input.type", "expected integer"))
	}
}

func (intShaper) Rehydrate(v any) core.Result {
	return intShaper{}.Coerce(v)
}

type boolShaper struct{}

func (boolShaper) Coerce(v any) core.Result {
	if v == nil {
		return core.Fail(core.NewCode("orm.input.null", "null value for bool field"))
	}
	b, ok := v.(bool)
	if !ok {
		return core.Fail(core.NewCode("orm.input.type", "expected bool"))
	}
	return core.Ok(b)
}

func (boolShaper) Rehydrate(v any) core.Result {
	// Rehydrate handles int→bool for SQLite compatibility
	if v == nil {
		return core.Fail(core.NewCode("orm.output.type", "null for bool field"))
	}
	switch val := v.(type) {
	case bool:
		return core.Ok(val)
	case int64:
		return core.Ok(val != 0)
	case int:
		return core.Ok(val != 0)
	case float64:
		return core.Ok(val != 0)
	case string:
		return stringToBool(val)
	default:
		return core.Fail(core.NewCode("orm.output.type", "expected bool-compatible value"))
	}
}

type floatShaper struct{}

func (floatShaper) Coerce(v any) core.Result {
	switch val := v.(type) {
	case float64:
		return core.Ok(val)
	case int:
		return core.Ok(float64(val))
	case int64:
		return core.Ok(float64(val))
	case float32:
		return core.Ok(float64(val))
	default:
		return core.Fail(core.NewCode("orm.input.type", "expected float"))
	}
}

func (floatShaper) Rehydrate(v any) core.Result {
	return floatShaper{}.Coerce(v)
}

type timeShaper struct{}

func (timeShaper) Coerce(v any) core.Result {
	switch val := v.(type) {
	case core.Time:
		return core.Ok(val)
	case string:
		// Try lenient parse — common formats
		layouts := []string{
			core.TimeRFC3339,
			core.TimeRFC3339Nano,
			core.TimeDateTime,
			core.TimeDateOnly,
			core.TimeRFC1123,
			core.TimeRFC822,
			core.TimeStamp,
			core.TimeKitchen,
		}
		for _, layout := range layouts {
			t := core.TimeParse(layout, val)
			if t.OK {
				return t
			}
		}
		return core.Fail(core.NewCode("orm.input.format", "invalid time format"))
	case int64:
		return core.Ok(core.UnixTime(val))
	default:
		return core.Fail(core.NewCode("orm.input.type", "expected time-compatible value"))
	}
}

func (timeShaper) Rehydrate(v any) core.Result {
	return timeShaper{}.Coerce(v)
}

type jsonShaper struct{}

func (jsonShaper) Coerce(v any) core.Result {
	data := core.JSONMarshal(v)
	if !data.OK {
		return core.Fail(core.NewCode("orm.input.coerce", "JSON marshal failed"))
	}
	return core.Ok(data.Value.([]byte))
}

func (jsonShaper) Rehydrate(v any) core.Result {
	switch val := v.(type) {
	case []byte:
		var target any
		ur := core.JSONUnmarshal(val, &target)
		if !ur.OK {
			return core.Fail(core.NewCode("orm.output.json", "JSON unmarshal failed"))
		}
		return core.Ok(target)
	case string:
		var target any
		ur := core.JSONUnmarshal([]byte(val), &target)
		if !ur.OK {
			return core.Fail(core.NewCode("orm.output.json", "JSON unmarshal failed"))
		}
		return core.Ok(target)
	default:
		return core.Fail(core.NewCode("orm.output.type", "expected JSON bytes or string"))
	}
}

type bytesShaper struct{}

func (bytesShaper) Coerce(v any) core.Result {
	switch val := v.(type) {
	case []byte:
		return core.Ok(val)
	case string:
		return core.Ok([]byte(val))
	default:
		return core.Fail(core.NewCode("orm.input.type", "expected []byte or string"))
	}
}

func (bytesShaper) Rehydrate(v any) core.Result {
	switch val := v.(type) {
	case []byte:
		return core.Ok(val)
	case string:
		return core.Ok([]byte(val))
	default:
		return core.Fail(core.NewCode("orm.output.type", "expected []byte"))
	}
}

// --- Cross-type coercers ---

// BoolToInt accepts bool, returns int64 (false=0, true=1).
func BoolToInt(v any) core.Result {
	if b, ok := v.(bool); ok {
		if b {
			return core.Ok(int64(1))
		}
		return core.Ok(int64(0))
	}
	return core.Fail(core.NewCode("orm.input.coerce", "BoolToInt: expected bool"))
}

// IntToBool accepts int, returns bool (0=false, else=true).
func IntToBool(v any) core.Result {
	switch val := v.(type) {
	case int64:
		return core.Ok(val != 0)
	case int:
		return core.Ok(val != 0)
	case float64:
		return core.Ok(val != 0)
	default:
		return core.Fail(core.NewCode("orm.input.coerce", "IntToBool: expected numeric"))
	}
}

// StringToInt accepts numeric string, returns int64.
func StringToInt(v any) core.Result {
	if s, ok := v.(string); ok {
		parsedR := core.ParseInt(s, 10, 64)
		if !parsedR.OK {
			return core.Fail(core.NewCode("orm.input.coerce", "StringToInt: invalid numeric string"))
		}
		return core.Ok(parsedR.Value.(int64))
	}
	return core.Fail(core.NewCode("orm.input.coerce", "StringToInt: expected string"))
}

// IntToString accepts int, returns string.
func IntToString(v any) core.Result {
	switch val := v.(type) {
	case int64:
		return core.Ok(core.Itoa(int(val)))
	case int:
		return core.Ok(core.Itoa(val))
	case float64:
		return core.Ok(core.Itoa(int(val)))
	default:
		return core.Fail(core.NewCode("orm.input.coerce", "IntToString: expected numeric"))
	}
}

// StringToBool accepts "true"/"false"/"1"/"0"/"yes"/"no"/"on"/"off", returns bool.
func StringToBool(v any) core.Result {
	return stringToBool(v)
}

func stringToBool(v any) core.Result {
	if s, ok := v.(string); ok {
		switch core.Lower(s) {
		case "true", "1", "yes", "on":
			return core.Ok(true)
		case "false", "0", "no", "off", "":
			return core.Ok(false)
		default:
			return core.Fail(core.NewCode("orm.input.coerce", "StringToBool: expected truthy/falsy string"))
		}
	}
	return core.Fail(core.NewCode("orm.input.coerce", "StringToBool: expected string"))
}

// TimeToUnix accepts core.Time, returns int64 (Unix seconds).
func TimeToUnix(v any) core.Result {
	if t, ok := v.(core.Time); ok {
		return core.Ok(t.Unix())
	}
	return core.Fail(core.NewCode("orm.input.coerce", "TimeToUnix: expected core.Time"))
}

// UnixToTime accepts int64, returns core.Time.
func UnixToTime(v any) core.Result {
	switch val := v.(type) {
	case int64:
		return core.Ok(core.UnixTime(val))
	case int:
		return core.Ok(core.UnixTime(int64(val)))
	case float64:
		return core.Ok(core.UnixTime(int64(val)))
	default:
		return core.Fail(core.NewCode("orm.input.coerce", "UnixToTime: expected int64"))
	}
}

// --- Shaper registry ---

var shaperRegistry = map[string]Shaper{
	"string":  stringShaper{},
	"int":     intShaper{},
	"int64":   intShaper{},
	"bool":    boolShaper{},
	"float":   floatShaper{},
	"float64": floatShaper{},
	"time":    timeShaper{},
	"json":    jsonShaper{},
	"bytes":   bytesShaper{},
}

var formatShapers = map[string]Shaper{
	"email":    emailShaper{},
	"url":      urlShaper{},
	"uuid":     uuidShaper{},
	"ipv4":     ipv4Shaper{},
	"ipv6":     ipv6Shaper{},
	"hostname": hostnameShaper{},
	"slug":     slugShaper{},
}

var namedCoercers = map[string]func(any) core.Result{
	"BoolToInt":    BoolToInt,
	"IntToBool":    IntToBool,
	"StringToInt":  StringToInt,
	"IntToString":  IntToString,
	"StringToBool": StringToBool,
	"TimeToUnix":   TimeToUnix,
	"UnixToTime":   UnixToTime,
}

// Apply runs the full validation chain for a field on an input value:
// Coerce → type-shaper → NotNull → Format → Min/Max → Pattern → OneOf
func Apply(field Field, value any) core.Result {
	// Step 1: Run coercer if declared
	if field.CoerceName != "" {
		if coerceFn, ok := namedCoercers[field.CoerceName]; ok {
			coerced := coerceFn(value)
			if !coerced.OK {
				return coerced
			}
			value = coerced.Value
		} else {
			return core.Fail(core.NewCode("orm.input.coerce", "unknown coercer: "+field.CoerceName))
		}
	}

	// Step 2: Type shaper
	shaper, ok := shaperRegistry[field.Type]
	if ok {
		coerced := shaper.Coerce(value)
		if !coerced.OK {
			return coerced
		}
		value = coerced.Value
	}

	// Step 3: NotNull check
	if hasConstraint(field, "notnull") {
		if isNilOrZero(value) {
			return core.Fail(core.NewCode("orm.input.null", "field cannot be null"))
		}
	}

	// Step 4: Format validation
	if field.Format != "" {
		if formatShaper, ok := formatShapers[field.Format]; ok {
			coerced := formatShaper.Coerce(value)
			if !coerced.OK {
				return coerced
			}
			value = coerced.Value
		}
	}

	// Step 5: Min/Max bounds
	if field.Min != nil {
		r := checkMin(value, *field.Min)
		if !r.OK {
			return r
		}
	}
	if field.Max != nil {
		r := checkMax(value, *field.Max)
		if !r.OK {
			return r
		}
	}

	// Step 6: Pattern validation
	if field.Pattern != "" {
		r := checkPattern(value, field.Pattern)
		if !r.OK {
			return r
		}
	}

	// Step 7: OneOf validation
	if len(field.OneOf) > 0 {
		r := checkOneOf(value, field.OneOf)
		if !r.OK {
			return r
		}
	}

	// Step 8: MaxBytes for blob fields
	if field.MaxBytes != nil {
		r := checkMaxBytes(value, *field.MaxBytes)
		if !r.OK {
			return r
		}
	}

	return core.Ok(value)
}

// RehydrateApply runs the output rehydration chain for a field:
// type-shaper Rehydrate → Format validation → Min/Max → Pattern
func RehydrateApply(field Field, value any) core.Result {
	if value == nil {
		if hasConstraint(field, "notnull") {
			return core.Fail(core.NewCode("orm.output.type", "null value for notnull field"))
		}
		return core.Ok(nil)
	}

	shaper, ok := shaperRegistry[field.Type]
	if ok {
		rehydrated := shaper.Rehydrate(value)
		if !rehydrated.OK {
			return rehydrated
		}
		value = rehydrated.Value
	}

	if field.Format != "" {
		if formatShaper, ok := formatShapers[field.Format]; ok {
			coerced := formatShaper.Rehydrate(value)
			if !coerced.OK {
				return coerced
			}
			value = coerced.Value
		}
	}

	return core.Ok(value)
}

// --- Helpers ---

func hasConstraint(field Field, c string) bool {
	return slices.Contains(field.Constraints, c)
}

func isNilOrZero(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case int64:
		return val == 0
	case int:
		return val == 0
	case float64:
		return val == 0
	case bool:
		return false
	case []byte:
		return len(val) == 0
	}
	return false
}

func checkMin(v any, min int64) core.Result {
	switch val := v.(type) {
	case string:
		if int64(len(val)) < min {
			return core.Fail(core.NewCode("orm.input.range", "value below minimum length"))
		}
	case int64:
		if val < min {
			return core.Fail(core.NewCode("orm.input.range", "value below minimum"))
		}
	case int:
		if int64(val) < min {
			return core.Fail(core.NewCode("orm.input.range", "value below minimum"))
		}
	case float64:
		if val < float64(min) {
			return core.Fail(core.NewCode("orm.input.range", "value below minimum"))
		}
	}
	return core.Ok(v)
}

func checkMax(v any, max int64) core.Result {
	switch val := v.(type) {
	case string:
		if int64(len(val)) > max {
			return core.Fail(core.NewCode("orm.input.range", "value above maximum length"))
		}
	case int64:
		if val > max {
			return core.Fail(core.NewCode("orm.input.range", "value above maximum"))
		}
	case int:
		if int64(val) > max {
			return core.Fail(core.NewCode("orm.input.range", "value above maximum"))
		}
	case float64:
		if val > float64(max) {
			return core.Fail(core.NewCode("orm.input.range", "value above maximum"))
		}
	}
	return core.Ok(v)
}

func checkPattern(v any, pattern string) core.Result {
	s, ok := v.(string)
	if !ok {
		return core.Ok(v)
	}
	re := core.Regex(pattern)
	if !re.OK {
		return core.Fail(core.NewCode("orm.input.pattern", "invalid regex pattern"))
	}
	if !re.Value.(*core.Regexp).MatchString(s) {
		return core.Fail(core.NewCode("orm.input.pattern", "value does not match pattern"))
	}
	return core.Ok(v)
}

func checkOneOf(v any, vals []any) core.Result {
	for _, allowed := range vals {
		if core.DeepEqual(v, allowed) {
			return core.Ok(v)
		}
	}
	return core.Fail(core.NewCode("orm.input.oneof", "value not in allowed set"))
}

func checkMaxBytes(v any, max int64) core.Result {
	if b, ok := v.([]byte); ok {
		if int64(len(b)) > max {
			return core.Fail(core.NewCode("orm.input.range", "value exceeds max bytes"))
		}
	}
	if s, ok := v.(string); ok {
		if int64(len(s)) > max {
			return core.Fail(core.NewCode("orm.input.range", "value exceeds max bytes"))
		}
	}
	return core.Ok(v)
}
