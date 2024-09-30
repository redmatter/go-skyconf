package skyconf

import (
	"context"
	"errors"
	"fmt"
	ssmpkg "github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Source can format a parameter name and fetch a set of parameters from a source.
type Source interface {
	// Source fetches the parameters from the source.
	Source(ctx context.Context, params []string) (values map[string]string, err error)
	// ParameterName formats the parameter name.
	ParameterName(parts []string) string
	// ID returns the ID of the source.
	ID() string
}

// ParseSSM parses configuration from AWS SSM into the provided struct, using parameters from the provided path.
func ParseSSM(ctx context.Context, ssm *ssmpkg.Client, path string, cfg interface{}) (err error) {
	return Parse(ctx, cfg, false, SSMSource(ssm, path))
}

var ErrNoSource = errors.New("no sources provided")
var ErrGetParameters = errors.New("failed to get parameters")
var ErrBadFieldValue = errors.New("failed to set value for field")
var ErrBadDefaultFieldValue = errors.New("failed to set default value for field")
var ErrParameterNotFound = errors.New("parameter not found in source")

// Parse parses configuration into the provided struct.
func Parse(ctx context.Context, cfg interface{}, withUntagged bool, sources ...Source) (err error) {
	if len(sources) == 0 {
		err = ErrNoSource
		return
	}

	// Get the list of fields from the configuration struct to process.
	var fields []fieldInfo
	fields, err = extractFields(withUntagged, nil, cfg, fieldOptions{})
	if err != nil {
		err = fmt.Errorf("failed to extract fields: %w", err)
		return
	}

	// First, process any default values for the fields
	for _, field := range fields {
		if field.options.defaultValue == "" {
			continue
		}

		err = processFieldValue(true, field.options.defaultValue, field.structField)
		if err != nil {
			err = fmt.Errorf("%w of type %s: %w", ErrBadDefaultFieldValue, field.structField.Type(), err)
			return
		}
	}

	// Format the keys for each field based on the source by matching the source ID.
	for sourceIdx, source := range sources {
		var keys []string
		var fieldsMap = make(map[string]fieldInfo)
		for _, field := range fields {
			if field.options.source == "" || field.options.source == source.ID() {
				key := source.ParameterName(field.nameParts)
				keys = append(keys, key)
				fieldsMap[key] = field
			}
		}

		// Fetch the parameters
		var paramMap map[string]string
		paramMap, err = source.Source(ctx, keys)
		if err != nil {
			err = fmt.Errorf("%w from source '%s' : %w", ErrGetParameters, source.ID(), err)
			return
		}

		// Process the fields based on the values obtained from the source
		for key, field := range fieldsMap {
			value, ok := paramMap[key]

			// If the field is not found in the source, check if it is optional
			if !ok {
				// If a source is not specified and the current source is not the last source, continue
				if field.options.source == "" && sourceIdx < len(sources)-1 {
					continue
				}

				// If the field is non-zero value, continue
				// The field might have a non-zero value set by the default value or a previous source or from the struct initialisation.
				if !field.structField.IsZero() {
					continue
				}

				// If the field is optional, continue
				if field.options.optional {
					continue
				}

				// If the field is not optional, and no default value is provided, return an error
				if field.options.source == "" && len(sources) > 1 {
					err = fmt.Errorf("%w - (any):%s", ErrParameterNotFound, key)
				} else {
					// If the source is specified, return an error with the source ID
					err = fmt.Errorf("%w - %s:%s", ErrParameterNotFound, source.ID(), key)
				}

				return
			}

			// Process the field using the value obtained from the source
			if err = processFieldValue(false, value, field.structField); err != nil {
				err = fmt.Errorf("%w of type %s: %w", ErrBadFieldValue, field.structField.Type(), err)
				return
			}
		}
	}

	return
}
