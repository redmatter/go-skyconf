package skyconf

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Setter is implemented by types can self-deserialize values.
type Setter interface {
	Set(value string) error
}

type fieldInfo struct {
	nameParts   []string
	structField reflect.Value
	options     fieldOptions
}

type fieldOptions struct {
	defaultValue string
	optional     bool
	flatten      bool
	source       string
}

// inherit copies the options from the parent.
func (o *fieldOptions) inherit(parent fieldOptions) {
	o.source = parent.source
}

var ErrInvalidStruct = errors.New("config must be a pointer to a struct")
var ErrBadTags = errors.New("error parsing tags for field")

// extractFields uses reflection to examine the struct and extract the fields.
func extractFields(withUntagged bool, prefix []string, target interface{}, parentOptions fieldOptions) (fields []fieldInfo, err error) {
	if prefix == nil {
		prefix = []string{}
	}

	s := reflect.ValueOf(target)

	// Make sure the config is a pointer to struct
	if s.Kind() != reflect.Ptr {
		return nil, ErrInvalidStruct
	}
	if s = s.Elem(); s.Kind() != reflect.Struct {
		return nil, ErrInvalidStruct
	}

	targetType := s.Type()

	// Iterate over the fields of the struct.
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)

		// Skip unexported fields.
		if !f.CanSet() {
			continue
		}

		structField := targetType.Field(i)

		// Get the 'sky' tag.
		tags, tagged := structField.Tag.Lookup("sky")

		// If there is no tag (not even an empty tag), ignore the field if withUntagged == false
		if !tagged && !withUntagged {
			continue
		}

		// If explicitly ignored using "-", ignore the field.
		if tags == "-" {
			continue
		}

		fieldName := structField.Name

		// Put together the field key and options.
		var options fieldOptions
		var keyPart string
		keyPart, options, err = parseTag(tags, parentOptions)
		if err != nil {
			err = fmt.Errorf("%w %s: %s", ErrBadTags, fieldName, err)
			return
		}

		// If the key part is empty, use the field name. They will be formatted and joined later by a parameter source.
		if keyPart == "" {
			keyPart = fieldName
		}

		// Make the field key by appending the field key part to the prefix.
		// This might be ignored if the field is flattened.
		fieldKey := append(prefix, keyPart)

		// If the field is a pointer, and it's nil, create a new instance.
		// Iterate over the pointer until we get to the actual struct.
		for f.Kind() == reflect.Ptr {
			if f.IsNil() {
				// If the field is not a struct, we can't zero it out.
				if f.Type().Elem().Kind() != reflect.Struct {
					break
				}

				// Initialize the pointer with a new instance.
				f.Set(reflect.New(f.Type().Elem()))
			}

			// Drill down to the next level.
			f = f.Elem()
		}

		switch {

		// If the field is a struct, and it's not a Setter, TextUnmarshaler, or BinaryUnmarshaler, i.e. it can't
		// deserialize itself, recursively extract fields, appending the field key as we go.
		case f.Kind() == reflect.Struct &&
			setterFrom(f) == nil && textUnmarshaler(f) == nil && binaryUnmarshaler(f) == nil:

			// If the field is anonymous, and it's set to flatten, we don't want to append the field key part.
			innerPrefix := fieldKey
			if structField.Anonymous || options.flatten {
				innerPrefix = prefix
			}

			embeddedPtr := f.Addr().Interface()

			// Recursively extract fields from the embedded struct.
			var innerFields []fieldInfo
			innerFields, err = extractFields(withUntagged, innerPrefix, embeddedPtr, options)
			if err != nil {
				return
			}

			// Append the inner fields to the list of fields.
			fields = append(fields, innerFields...)

		default:
			// Append the field to the list of fields.
			fields = append(fields, fieldInfo{
				nameParts:   fieldKey,
				structField: f,
				options:     options,
			})
		}
	}

	return fields, nil
}

func parseTag(tag string, parentOptions fieldOptions) (key string, f fieldOptions, err error) {
	if tag == "" {
		return
	}

	// Inherit the parent options.
	f.inherit(parentOptions)

	// Split the tag into parts.
	parts := strings.Split(tag, ",")

	// The first part is the key.
	key = parts[0]
	if len(parts) == 1 {
		return
	}

	// Process the options.
	for _, part := range parts[1:] {
		// Split the part into key and value.
		vals := strings.SplitN(part, ":", 2)
		prop := vals[0]

		switch len(vals) {
		case 1:
			switch prop {
			case "optional":
				f.optional = true
			case "flatten":
				f.flatten = true
			}
		case 2:
			val := strings.TrimSpace(vals[1])
			if val == "" {
				err = fmt.Errorf("tag %q missing a value", prop)
				return
			}
			switch prop {
			case "default":
				f.defaultValue = val
			case "source":
				f.source = val
			}
		}
	}

	return
}

// processFieldValue sets the value of a field based on its type.
func processFieldValue(isDefaultValue bool, value string, field reflect.Value) (err error) {
	t := field.Type()

	// If the field is a pointer, dereference it.
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if field.IsNil() {
			field.Set(reflect.New(t))
		}

		field = field.Elem()
	}

	// If the field is a zero value, and the value is the default value, skip it.
	if isDefaultValue && !field.IsZero() {
		return nil
	}

	// If it implements the Setter interface, use it.
	if setter := setterFrom(field); setter != nil {
		return setter.Set(value)
	}

	// If it implements the TextUnmarshaler use it.
	if tu := textUnmarshaler(field); tu != nil {
		return tu.UnmarshalText([]byte(value))
	}

	// If it implements the BinaryUnmarshaler use it.
	if bu := binaryUnmarshaler(field); bu != nil {
		return bu.UnmarshalBinary([]byte(value))
	}

	// Process the value based on the type of the field.
	switch t.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var val int64
		// If the field is a time.Duration, parse the duration.
		if field.Kind() == reflect.Int64 && t.PkgPath() == "time" && t.Name() == "Duration" {
			var d time.Duration
			d, err = time.ParseDuration(value)
			val = int64(d)
		} else {
			// Otherwise, parse the integer.
			val, err = strconv.ParseInt(value, 0, t.Bits())
		}

		if err == nil { // if no error
			field.SetInt(val)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Parse the unsigned integer.
		var val uint64
		val, err = strconv.ParseUint(value, 0, t.Bits())
		if err == nil { // if no error
			field.SetUint(val)
		}

	case reflect.Bool:
		// Parse the boolean.
		var val bool
		val, err = strconv.ParseBool(value)
		if err == nil { // if no error
			field.SetBool(val)
		}

	case reflect.Float32, reflect.Float64:
		// Parse the float.
		var val float64
		val, err = strconv.ParseFloat(value, t.Bits())
		if err == nil { // if no error
			field.SetFloat(val)
		}

	case reflect.Slice:
		// Split the value into parts and load them into the slice.
		vals := strings.Split(value, ";")
		sl := reflect.MakeSlice(t, len(vals), len(vals))
		for i, val := range vals {
			err = processFieldValue(false, val, sl.Index(i))
			if err != nil {
				return
			}
		}

		field.Set(sl)

	case reflect.Map:
		// Split the value into pairs and load them into the map.
		mp := reflect.MakeMap(t)
		if len(strings.TrimSpace(value)) != 0 {
			pairs := strings.Split(value, ";")
			for _, pair := range pairs {
				kvpair := strings.Split(pair, ":")
				if len(kvpair) != 2 {
					err = fmt.Errorf("invalid map item: %q", pair)
					return
				}

				k := reflect.New(t.Key()).Elem()
				err = processFieldValue(false, kvpair[0], k)
				if err != nil {
					return
				}

				v := reflect.New(t.Elem()).Elem()
				err = processFieldValue(false, kvpair[1], v)
				if err != nil {
					return
				}

				mp.SetMapIndex(k, v)
			}
		}

		field.Set(mp)

	default:
		err = fmt.Errorf("unexpected type %s when processing values", field.Type())
	}

	return
}

func interfaceFrom(field reflect.Value, fn func(interface{}, *bool)) {
	if !field.CanInterface() {
		return
	}

	var ok bool
	fn(field.Interface(), &ok)
	if !ok && field.CanAddr() {
		fn(field.Addr().Interface(), &ok)
	}
}

// setterFrom gets Setter from the field.
func setterFrom(field reflect.Value) (s Setter) {
	interfaceFrom(field, func(v interface{}, ok *bool) { s, *ok = v.(Setter) })
	return s
}

// textUnmarshaler gets encoding.TextUnmarshaler from the field.
func textUnmarshaler(field reflect.Value) (t encoding.TextUnmarshaler) {
	interfaceFrom(field, func(v interface{}, ok *bool) { t, *ok = v.(encoding.TextUnmarshaler) })
	return t
}

// binaryUnmarshaler gets encoding.BinaryUnmarshaler from the field.
func binaryUnmarshaler(field reflect.Value) (b encoding.BinaryUnmarshaler) {
	interfaceFrom(field, func(v interface{}, ok *bool) { b, *ok = v.(encoding.BinaryUnmarshaler) })
	return b
}
