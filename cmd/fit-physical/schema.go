package main

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
)

// writeSchema prints the shape of the JSON report and nothing else.
//
// It exists because finding where a field lives in a fit report otherwise means
// dumping keys out of a 900 kB file with a throwaway script — which is how it
// was done, repeatedly, and the reports are large enough that the answer is
// several minutes away rather than several seconds. The shape is the one thing
// about a report that is knowable without having a report.
//
// Derived from the Go types by reflection rather than written out by hand, for
// the usual reason a hand-written schema is worse than none: a field added to
// Report would not appear here, and a reader who trusted it would conclude the
// field does not exist.
func writeSchema(out io.Writer) {
	_, _ = fmt.Fprint(out,
		"fit-physical -o writes one JSON object per run, of this shape.\n"+
			"Types are the JSON ones; [] marks an array, and (omitempty) a field\n"+
			"absent from the object when it holds its zero value.\n\n")

	describeType(out, reflect.TypeOf(Report{}), 0, nil)
}

// describeType prints one struct's fields, one per line, recursing into the
// structs among them.
//
// seen carries the types on the path from the root rather than every type
// already printed. A type reached twice by two different routes is printed twice
// — Baseline and Best are both a Candidate and a reader looking under "best"
// should find its fields there — while a type that contains itself is cut off
// once, which is what stops the walk rather than an arbitrary depth limit.
func describeType(out io.Writer, structType reflect.Type, depth int, seen []reflect.Type) {
	for index := range structType.NumField() {
		field := structType.Field(index)
		if !field.IsExported() {
			continue
		}

		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}

		// An embedded struct with no name of its own is flattened into its
		// parent by encoding/json, so it is flattened here too: printing it as
		// a field would describe a nesting level the JSON does not have.
		if field.Anonymous && name == "" && field.Type.Kind() == reflect.Struct {
			describeType(out, field.Type, depth, seen)

			continue
		}

		if name == "" {
			name = field.Name
		}

		element, array := elementType(field.Type)
		if array {
			name += "[]"
		}

		note := ""
		if strings.Contains(options, "omitempty") {
			note = " (omitempty)"
		}

		_, _ = fmt.Fprintf(out, "%-*s%-*s %s%s\n",
			depth*2, "", max(1, 44-depth*2), name, jsonKind(element), note)

		if element.Kind() != reflect.Struct {
			continue
		}

		if slices.Contains(seen, element) {
			_, _ = fmt.Fprintf(out, "%*s  (recursive: same shape as above)\n", depth*2, "")

			continue
		}

		describeType(out, element, depth+1, append(seen, structType))
	}
}

// elementType unwraps pointers, slices and maps down to the type a reader will
// actually see fields on, and reports whether it arrives as an array.
//
// A map is not an array and its keys are not a schema, so a map is unwrapped to
// its value type without the [] — Report's fixedParams is an object keyed by
// parameter name, and saying so is more use than naming the key type.
func elementType(fieldType reflect.Type) (reflect.Type, bool) {
	array := false

	for {
		switch fieldType.Kind() {
		case reflect.Pointer:
			fieldType = fieldType.Elem()
		case reflect.Slice, reflect.Array:
			array = true
			fieldType = fieldType.Elem()
		case reflect.Map:
			fieldType = fieldType.Elem()
		default:
			return fieldType, array
		}
	}
}

func jsonKind(fieldType reflect.Type) string {
	switch fieldType.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Struct:
		return "object   " + fieldType.Name()
	case reflect.Interface:
		return "any"
	default:
		// Everything left in these types is a number: the report holds no
		// complex, channel or function fields and would not be encodable if it
		// did.
		return "number"
	}
}
