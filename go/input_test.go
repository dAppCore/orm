// SPDX-License-Identifier: EUPL-1.2

package orm_test

import (
	. "dappco.re/go"
	"dappco.re/go/orm"
)

func field(typ, name string) orm.Field {
	return orm.Field{Name: name, Type: typ}
}

func fieldNotNull(typ, name string) orm.Field {
	f := field(typ, name)
	f.Constraints = append(f.Constraints, "notnull")
	return f
}

func fieldFormat(typ, name, format string) orm.Field {
	f := field(typ, name)
	f.Format = format
	return f
}

func fieldMin(typ, name string, min int64) orm.Field {
	f := field(typ, name)
	f.Min = &min
	return f
}

func fieldMax(typ, name string, max int64) orm.Field {
	f := field(typ, name)
	f.Max = &max
	return f
}

func fieldOneOf(typ, name string, vals ...string) orm.Field {
	f := field(typ, name)
	f.OneOf = make([]any, len(vals))
	for i, v := range vals {
		f.OneOf[i] = v
	}
	return f
}

func fieldPattern(typ, name, pattern string) orm.Field {
	f := field(typ, name)
	f.Pattern = pattern
	return f
}

func fieldCoerce(typ, name, coerceName string) orm.Field {
	f := field(typ, name)
	f.CoerceName = coerceName
	return f
}

func fieldMaxBytes(typ, name string, n int64) orm.Field {
	f := field(typ, name)
	f.MaxBytes = &n
	return f
}

func TestInput_String_Good(t *T) {
	f := field("string", "name")
	r := orm.Apply(f, "snider")
	AssertTrue(t, r.OK)
	AssertEqual(t, "snider", r.Value.(string))
}

func TestInput_String_Bad(t *T) {
	f := field("string", "name")
	r := orm.Apply(f, 42)
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.type", r.Code())
}

func TestInput_String_Ugly(t *T) {
	f := field("string", "name")
	r := orm.Apply(f, nil)
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.null", r.Code())
}

func TestInput_Email_Good(t *T) {
	f := fieldFormat("string", "email", "email")
	r := orm.Apply(f, "snider@host.uk")
	AssertTrue(t, r.OK)
	AssertEqual(t, "snider@host.uk", r.Value.(string))
}

func TestInput_Email_Bracket_Good(t *T) {
	f := fieldFormat("string", "email", "email")
	r := orm.Apply(f, "John Smith <john@host.uk>")
	AssertTrue(t, r.OK)
	AssertEqual(t, "john@host.uk", r.Value.(string))
}

func TestInput_Email_Bad(t *T) {
	f := fieldFormat("string", "email", "email")
	r := orm.Apply(f, "not-an-email")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.format", r.Code())
}

func TestInput_URL_Good(t *T) {
	f := fieldFormat("string", "url", "url")
	r := orm.Apply(f, "https://forge.lthn.sh/repo")
	AssertTrue(t, r.OK)
}

func TestInput_URL_Bad(t *T) {
	f := fieldFormat("string", "url", "url")
	r := orm.Apply(f, "not a url")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.format", r.Code())
}

func TestInput_UUID_Good(t *T) {
	f := fieldFormat("string", "uuid", "uuid")
	r := orm.Apply(f, "550e8400-e29b-41d4-a716-446655440000")
	AssertTrue(t, r.OK)
}

func TestInput_UUID_Bad(t *T) {
	f := fieldFormat("string", "uuid", "uuid")
	r := orm.Apply(f, "not-a-uuid")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.format", r.Code())
}

func TestInput_IPv4_Good(t *T) {
	f := fieldFormat("string", "ip", "ipv4")
	r := orm.Apply(f, "192.168.1.1")
	AssertTrue(t, r.OK)
}

func TestInput_IPv4_Bad(t *T) {
	f := fieldFormat("string", "ip", "ipv4")
	r := orm.Apply(f, "not-an-ip")
	AssertTrue(t, !r.OK)
}

func TestInput_IPv6_Good(t *T) {
	f := fieldFormat("string", "ip", "ipv6")
	r := orm.Apply(f, "::1")
	AssertTrue(t, r.OK)
}

func TestInput_IPv6_Bad(t *T) {
	f := fieldFormat("string", "ip", "ipv6")
	r := orm.Apply(f, "192.168.1.1")
	AssertTrue(t, !r.OK)
}

func TestInput_Hostname_Good(t *T) {
	f := fieldFormat("string", "host", "hostname")
	r := orm.Apply(f, "forge.lthn.sh")
	AssertTrue(t, r.OK)
}

func TestInput_Hostname_Bad(t *T) {
	f := fieldFormat("string", "host", "hostname")
	r := orm.Apply(f, "not a hostname!!!")
	AssertTrue(t, !r.OK)
}

func TestInput_Slug_Good(t *T) {
	f := fieldFormat("string", "slug", "slug")
	r := orm.Apply(f, "Hello World!!")
	AssertTrue(t, r.OK)
	AssertEqual(t, "hello-world", r.Value.(string))
}

func TestInput_Slug_Bad(t *T) {
	f := fieldFormat("string", "slug", "slug")
	r := orm.Apply(f, 123)
	AssertTrue(t, !r.OK)
}

func TestInput_Int_Good(t *T) {
	f := field("int", "count")
	r := orm.Apply(f, int64(42))
	AssertTrue(t, r.OK)
	AssertEqual(t, int64(42), r.Value.(int64))
}

func TestInput_Int_Bad(t *T) {
	f := field("int", "count")
	r := orm.Apply(f, "not-a-number")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.type", r.Code())
}

func TestInput_Bool_Good(t *T) {
	f := field("bool", "active")
	r := orm.Apply(f, true)
	AssertTrue(t, r.OK)
	AssertEqual(t, true, r.Value.(bool))
}

func TestInput_Bool_Bad(t *T) {
	f := field("bool", "active")
	r := orm.Apply(f, "not-a-bool")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.type", r.Code())
}

func TestInput_Float_Good(t *T) {
	f := field("float64", "score")
	r := orm.Apply(f, float64(3.14))
	AssertTrue(t, r.OK)
	AssertEqual(t, float64(3.14), r.Value.(float64))
}

func TestInput_Float_Bad(t *T) {
	f := field("float64", "score")
	r := orm.Apply(f, "not-a-float")
	AssertTrue(t, !r.OK)
}

func TestInput_NotNull_Good(t *T) {
	f := fieldNotNull("string", "name")
	r := orm.Apply(f, "snider")
	AssertTrue(t, r.OK)
}

func TestInput_NotNull_Bad(t *T) {
	f := fieldNotNull("string", "name")
	r := orm.Apply(f, "")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.null", r.Code())
}

func TestInput_Min_Good(t *T) {
	f := fieldMin("int", "age", 18)
	r := orm.Apply(f, int64(25))
	AssertTrue(t, r.OK)
}

func TestInput_Min_Bad(t *T) {
	f := fieldMin("int", "age", 18)
	r := orm.Apply(f, int64(10))
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.range", r.Code())
}

func TestInput_Max_Bad(t *T) {
	f := fieldMax("int", "age", 150)
	r := orm.Apply(f, int64(200))
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.range", r.Code())
}

func TestInput_OneOf_Good(t *T) {
	f := fieldOneOf("string", "tier", "free", "pro", "enterprise")
	r := orm.Apply(f, "pro")
	AssertTrue(t, r.OK)
}

func TestInput_OneOf_Bad(t *T) {
	f := fieldOneOf("string", "tier", "free", "pro", "enterprise")
	r := orm.Apply(f, "platinum")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.oneof", r.Code())
}

func TestInput_Pattern_Good(t *T) {
	f := fieldPattern("string", "code", `^[A-Z]{3}-\d{4}$`)
	r := orm.Apply(f, "ABC-1234")
	AssertTrue(t, r.OK)
}

func TestInput_Pattern_Bad(t *T) {
	f := fieldPattern("string", "code", `^[A-Z]{3}-\d{4}$`)
	r := orm.Apply(f, "bad-format")
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.pattern", r.Code())
}

func TestInput_Coerce_BoolToInt_Good(t *T) {
	f := fieldCoerce("int", "active", "BoolToInt")
	r := orm.Apply(f, true)
	AssertTrue(t, r.OK)
	AssertEqual(t, int64(1), r.Value.(int64))
}

func TestInput_Coerce_IntToBool_Good(t *T) {
	f := fieldCoerce("bool", "flag", "IntToBool")
	r := orm.Apply(f, int64(1))
	AssertTrue(t, r.OK)
	AssertEqual(t, true, r.Value.(bool))
}

func TestInput_Coerce_StringToInt_Good(t *T) {
	f := fieldCoerce("int", "count", "StringToInt")
	r := orm.Apply(f, "42")
	AssertTrue(t, r.OK)
	AssertEqual(t, int64(42), r.Value.(int64))
}

func TestInput_Coerce_IntToString_Good(t *T) {
	f := fieldCoerce("string", "code", "IntToString")
	r := orm.Apply(f, int64(42))
	AssertTrue(t, r.OK)
	AssertEqual(t, "42", r.Value.(string))
}

func TestInput_Coerce_StringToBool_Good(t *T) {
	f := fieldCoerce("bool", "flag", "StringToBool")
	r := orm.Apply(f, "true")
	AssertTrue(t, r.OK)
	AssertEqual(t, true, r.Value.(bool))
}

func TestInput_Coerce_TimeToUnix_Good(t *T) {
	f := fieldCoerce("int64", "ts", "TimeToUnix")
	now := Now()
	r := orm.Apply(f, now)
	AssertTrue(t, r.OK)
	AssertEqual(t, now.Unix(), r.Value.(int64))
}

func TestInput_Coerce_UnixToTime_Good(t *T) {
	f := fieldCoerce("time", "created", "UnixToTime")
	ts := int64(1700000000)
	r := orm.Apply(f, ts)
	AssertTrue(t, r.OK)
}

func TestInput_Rehydrate_BoolFromInt_Good(t *T) {
	f := field("bool", "active")
	r := orm.RehydrateApply(f, int64(1))
	AssertTrue(t, r.OK)
	AssertEqual(t, true, r.Value.(bool))
}

func TestInput_Rehydrate_BoolFromZero_Good(t *T) {
	f := field("bool", "active")
	r := orm.RehydrateApply(f, int64(0))
	AssertTrue(t, r.OK)
	AssertEqual(t, false, r.Value.(bool))
}

func TestInput_Rehydrate_String_Good(t *T) {
	f := field("string", "name")
	r := orm.RehydrateApply(f, "snider")
	AssertTrue(t, r.OK)
	AssertEqual(t, "snider", r.Value.(string))
}

func TestInput_Rehydrate_Null_Good(t *T) {
	f := field("string", "optional")
	r := orm.RehydrateApply(f, nil)
	AssertTrue(t, r.OK)
	AssertEqual(t, nil, r.Value)
}

func TestInput_Rehydrate_NotNull_Bad(t *T) {
	f := fieldNotNull("string", "required")
	r := orm.RehydrateApply(f, nil)
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.output.type", r.Code())
}

func TestInput_MaxBytes_Good(t *T) {
	f := fieldMaxBytes("bytes", "data", 10)
	r := orm.Apply(f, []byte("hello"))
	AssertTrue(t, r.OK)
}

func TestInput_MaxBytes_Bad(t *T) {
	f := fieldMaxBytes("bytes", "data", 2)
	r := orm.Apply(f, []byte("too long"))
	AssertTrue(t, !r.OK)
	AssertEqual(t, "orm.input.range", r.Code())
}
