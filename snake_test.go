package skyconf

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestToSnakeCase(t *testing.T) {
	tests := map[string]string{
		"UUID":                           "uuid",
		"uuid":                           "uuid",
		"OtherUUID":                      "other_uuid",
		"otherUUID":                      "other_uuid",
		"Other_UUID":                     "other_uuid",
		"CoreAPIBaseURL":                 "core_api_base_url",
		"core_api_base_url":              "core_api_base_url",
		"/prefix_path/core_api_base_url": "/prefix_path/core_api_base_url",
	}

	for input, expected := range tests {
		assert.Equal(t, expected, ToSnakeCase(input))
	}
}
