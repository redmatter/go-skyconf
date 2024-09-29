package skyconf

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

func Test_parseTag(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		wantKey string
		wantF   fieldOptions
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "empty tag",
			tag:     "",
			wantKey: "",
			wantF:   fieldOptions{},
			wantErr: assert.NoError,
		},
		{
			name:    "key only tag",
			tag:     "key",
			wantKey: "key",
			wantF:   fieldOptions{},
			wantErr: assert.NoError,
		},
		{
			name:    "optional tag",
			tag:     ",optional",
			wantKey: "",
			wantF:   fieldOptions{optional: true},
			wantErr: assert.NoError,
		},
		{
			name:    "flatten tag",
			tag:     ",flatten",
			wantKey: "",
			wantF:   fieldOptions{flatten: true},
			wantErr: assert.NoError,
		},
		{
			name:    "default tag",
			tag:     ",default:default",
			wantKey: "",
			wantF:   fieldOptions{defaultValue: "default"},
			wantErr: assert.NoError,
		},
		{
			name:    "source tag",
			tag:     ",source:source",
			wantKey: "",
			wantF:   fieldOptions{source: "source"},
			wantErr: assert.NoError,
		},
		{
			name:    "optional,flatten,default,source tag",
			tag:     ",optional,flatten,default:default,source:source",
			wantKey: "",
			wantF:   fieldOptions{optional: true, flatten: true, defaultValue: "default", source: "source"},
			wantErr: assert.NoError,
		},
		{
			name:    "missing value",
			tag:     ",default:",
			wantErr: assert.Error,
		},
		{
			name:    "missing value",
			tag:     ",source:",
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotF, err := parseTag(tt.tag)
			if !tt.wantErr(t, err, "parseTag(%v)", tt.tag) {
				return
			}

			if err != nil {
				return
			}

			assert.Equalf(t, tt.wantKey, gotKey, "parseTag(%v)", tt.tag)
			assert.Equalf(t, tt.wantF, gotF, "parseTag(%v)", tt.tag)
		})
	}
}

type mockSetter string

func (m *mockSetter) Set(value string) error {
	*m = mockSetter(value)
	return nil
}

type mockSetterWithErr string

func (m *mockSetterWithErr) Set(_ string) error {
	return assert.AnError
}

type mockTextUnmarshaler string

func (m *mockTextUnmarshaler) UnmarshalText(text []byte) error {
	*m = mockTextUnmarshaler(text)
	return nil
}

type mockTextUnmarshalerWithErr string

func (m *mockTextUnmarshalerWithErr) UnmarshalText(_ []byte) error {
	return assert.AnError
}

type mockBinaryUnmarshaler string

func (m *mockBinaryUnmarshaler) UnmarshalBinary(data []byte) error {
	*m = mockBinaryUnmarshaler(data)
	return nil
}

type mockBinaryUnmarshalerWithErr string

func (m *mockBinaryUnmarshalerWithErr) UnmarshalBinary(_ []byte) error {
	return assert.AnError
}

func Test_processFieldValue(t *testing.T) {
	nonZeroString := new(string)
	*nonZeroString = "non-zero-value"

	tests := []struct {
		name           string
		isDefaultValue bool
		value          string
		field          reflect.Value
		expected       interface{}
		expectErr      bool
	}{
		{
			name:           "string field",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(string)).Elem(),
			expected:       "test",
			expectErr:      false,
		},
		{
			name:           "int field",
			isDefaultValue: false,
			value:          "123",
			field:          reflect.ValueOf(new(int)).Elem(),
			expected:       123,
			expectErr:      false,
		},
		{
			name:           "bool field",
			isDefaultValue: false,
			value:          "true",
			field:          reflect.ValueOf(new(bool)).Elem(),
			expected:       true,
			expectErr:      false,
		},
		{
			name:           "float field",
			isDefaultValue: false,
			value:          "1.23",
			field:          reflect.ValueOf(new(float64)).Elem(),
			expected:       1.23,
			expectErr:      false,
		},
		{
			name:           "duration field",
			isDefaultValue: false,
			value:          "1h",
			field:          reflect.ValueOf(new(time.Duration)).Elem(),
			expected:       time.Hour,
			expectErr:      false,
		},
		{
			name:           "slice field",
			isDefaultValue: false,
			value:          "a;b;c",
			field:          reflect.ValueOf(new([]string)).Elem(),
			expected:       []string{"a", "b", "c"},
			expectErr:      false,
		},
		{
			name:           "slice field with mismatched type",
			isDefaultValue: false,
			value:          "1;2;A",
			field:          reflect.ValueOf(new([]int)).Elem(),
			expected:       []string{},
			expectErr:      true,
		},
		{
			name:           "map field",
			isDefaultValue: false,
			value:          "key1:val1;key2:val2",
			field:          reflect.ValueOf(new(map[string]string)).Elem(),
			expected:       map[string]string{"key1": "val1", "key2": "val2"},
			expectErr:      false,
		},
		{
			name:           "map field with invalid entry",
			isDefaultValue: false,
			value:          "key1:val1;key2-val2",
			field:          reflect.ValueOf(new(map[string]string)).Elem(),
			expected:       map[string]string{},
			expectErr:      true,
		},
		{
			name:           "map field with invalid key",
			isDefaultValue: false,
			value:          "1:1;key2:2",
			field:          reflect.ValueOf(new(map[int]int)).Elem(),
			expected:       map[int]int{},
			expectErr:      true,
		},
		{
			name:           "map field with invalid value",
			isDefaultValue: false,
			value:          "key1:1;key2:val2",
			field:          reflect.ValueOf(new(map[string]int)).Elem(),
			expected:       map[string]int{},
			expectErr:      true,
		},
		{
			name:           "invalid int field",
			isDefaultValue: false,
			value:          "abc",
			field:          reflect.ValueOf(new(int)).Elem(),
			expected:       0,
			expectErr:      true,
		},
		{
			name:           "uint field",
			isDefaultValue: false,
			value:          "123",
			field:          reflect.ValueOf(new(uint)).Elem(),
			expected:       uint(123),
			expectErr:      false,
		},
		{
			name:           "uint8 field",
			isDefaultValue: false,
			value:          "255",
			field:          reflect.ValueOf(new(uint8)).Elem(),
			expected:       uint8(255),
			expectErr:      false,
		},
		{
			name:           "uint16 field",
			isDefaultValue: false,
			value:          "65535",
			field:          reflect.ValueOf(new(uint16)).Elem(),
			expected:       uint16(65535),
			expectErr:      false,
		},
		{
			name:           "uint32 field",
			isDefaultValue: false,
			value:          "4294967295",
			field:          reflect.ValueOf(new(uint32)).Elem(),
			expected:       uint32(4294967295),
			expectErr:      false,
		},
		{
			name:           "uint64 field",
			isDefaultValue: false,
			value:          "18446744073709551615",
			field:          reflect.ValueOf(new(uint64)).Elem(),
			expected:       uint64(18446744073709551615),
			expectErr:      false,
		},
		{
			name:           "invalid uint field",
			isDefaultValue: false,
			value:          "-1",
			field:          reflect.ValueOf(new(uint)).Elem(),
			expected:       uint(0),
			expectErr:      true,
		},
		{
			name:           "setter field",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockSetter)).Elem(),
			expected:       mockSetter("test"),
		},
		{
			name:           "setter field with error",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockSetterWithErr)).Elem(),
			expectErr:      true,
		},
		{
			name:           "text unmarshaler field",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockTextUnmarshaler)).Elem(),
			expected:       mockTextUnmarshaler("test"),
		},
		{
			name:           "text unmarshaler field with error",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockTextUnmarshalerWithErr)).Elem(),
			expectErr:      true,
		},
		{
			name:           "binary unmarshaler field",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockBinaryUnmarshaler)).Elem(),
			expected:       mockBinaryUnmarshaler("test"),
		},
		{
			name:           "binary unmarshaler field with error",
			isDefaultValue: false,
			value:          "test",
			field:          reflect.ValueOf(new(mockBinaryUnmarshalerWithErr)).Elem(),
			expectErr:      true,
		},
		{
			name:           "non-zero default value",
			isDefaultValue: true,
			value:          "test",
			field:          reflect.ValueOf(nonZeroString).Elem(),
			expected:       *nonZeroString,
		},
		{
			name:           "pointer field",
			isDefaultValue: false,
			value:          *nonZeroString,
			field:          reflect.ValueOf(new(*string)).Elem(),
			expected:       nonZeroString,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processFieldValue(tt.isDefaultValue, tt.value, tt.field)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				if assert.NoError(t, err) {
					assert.Equal(t, tt.expected, tt.field.Interface())
				}
			}
		})
	}
}

func Test_extractFields(t *testing.T) {
	prefix := []string{"prefix"}
	var target interface{}
	var err error

	// Tags with errors
	err = nil
	target = &struct {
		Field1 string `sky:"field1,source:"`
	}{}
	_, err = extractFields(true, nil, target)
	assert.Error(t, err)

	// Bad config; not a struct
	err = nil
	targetStr := "not a struct"
	target = &targetStr
	_, err = extractFields(true, nil, target)
	assert.Error(t, err)

	// Bad config; not a pointer to a struct
	err = nil
	target = struct {
		Field1 string `sky:"field1"`
	}{}
	_, err = extractFields(true, nil, target)
	assert.Error(t, err)

	// Uninitialised struct field will be initialised with zero value
	err = nil

	type AStruct struct {
		Field2 string `sky:"field2"`
	}
	type AConfig struct {
		Field1 string `sky:"field1"`
		A      *AStruct
	}

	target = &AConfig{}
	_, err = extractFields(true, nil, target)
	assert.NoError(t, err)
	assert.NotNil(t, target.(*AConfig).A)

	// A realistic example
	err = nil

	type Embedded1 struct {
		EmbeddedField1 string `sky:"embedded1_field1"`
		EmbeddedField2 int    `sky:"embedded1_field2"`
	}

	type Embedded2 struct {
		EmbeddedField1 string `sky:"field1"`
		EmbeddedField2 int    `sky:"field2"`
	}

	type ConfigStruct struct {
		Field1    string `sky:""`
		Field2    int    `sky:"field2,optional"`
		Field3    bool   `sky:"field3,flatten"`
		Field4    string `sky:"field4,default:default"`
		Field5    string `sky:"field5,source:source"`
		Field6    string
		Field7    int
		Field8    time.Duration
		Field9    int `sky:"-"`
		Embedded1 `sky:",flatten"`
		Embedded2 `sky:""`

		// Unexported fields are ignored
		field10 string `sky:"field10"`
	}

	target = &ConfigStruct{}

	// withUntagged = true
	gotFields, err := extractFields(true, prefix, target)
	assert.NoError(t, err)

	expectedFields := []fieldInfo{
		{
			nameParts:   []string{"prefix", "Field1"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{optional: true},
		},
		{
			nameParts:   []string{"prefix", "field3"},
			structField: reflect.ValueOf(false),
			options:     fieldOptions{flatten: true},
		},
		{
			nameParts:   []string{"prefix", "field4"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{defaultValue: "default"},
		},
		{
			nameParts:   []string{"prefix", "field5"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{source: "source"},
		},
		{
			nameParts:   []string{"prefix", "Field6"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Field7"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Field8"},
			structField: reflect.ValueOf(time.Duration(0)),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "embedded1_field1"},
			structField: reflect.ValueOf("string"),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "embedded1_field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Embedded2", "field1"},
			structField: reflect.ValueOf("string"),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Embedded2", "field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{},
		},
	}

	assert.Equal(t, len(expectedFields), len(gotFields))
	for i, expectedField := range expectedFields {
		assert.Equal(t, expectedField.nameParts, gotFields[i].nameParts)
		assert.Equal(t, expectedField.structField.Type(), gotFields[i].structField.Type())
		assert.Equal(t, expectedField.options, gotFields[i].options)
	}

	// withUntagged = false
	gotFields, err = extractFields(false, prefix, target)
	assert.NoError(t, err)

	expectedFields = []fieldInfo{
		{
			nameParts:   []string{"prefix", "Field1"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{optional: true},
		},
		{
			nameParts:   []string{"prefix", "field3"},
			structField: reflect.ValueOf(false),
			options:     fieldOptions{flatten: true},
		},
		{
			nameParts:   []string{"prefix", "field4"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{defaultValue: "default"},
		},
		{
			nameParts:   []string{"prefix", "field5"},
			structField: reflect.ValueOf(""),
			options:     fieldOptions{source: "source"},
		},
		{
			nameParts:   []string{"prefix", "embedded1_field1"},
			structField: reflect.ValueOf("string"),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "embedded1_field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Embedded2", "field1"},
			structField: reflect.ValueOf("string"),
			options:     fieldOptions{},
		},
		{
			nameParts:   []string{"prefix", "Embedded2", "field2"},
			structField: reflect.ValueOf(0),
			options:     fieldOptions{},
		},
	}

	assert.Equal(t, len(expectedFields), len(gotFields))
	for i, expectedField := range expectedFields {
		assert.Equal(t, expectedField.nameParts, gotFields[i].nameParts)
		assert.Equal(t, expectedField.structField.Type(), gotFields[i].structField.Type())
		assert.Equal(t, expectedField.options, gotFields[i].options)
	}
}
