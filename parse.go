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
	// Refreshable returns true if the source is refreshable.
	Refreshable() bool
	// ID returns the ID of the source.
	ID() string
}

// Refresher refreshes configuration at specified intervals.
type Refresher interface {
	// Refresh starts a new goroutine that updates the configuration at specified intervals until the context is
	// cancelled. It returns a channel that sends updated field IDs. If an error occurs, the provided error function is
	// called. If no error function is provided, the error is ignored.
	Refresh(ctx context.Context, ef func(err error)) <-chan string
	// RefreshOnce refreshes the configuration once, returning the first error that occurs.
	RefreshOnce(ctx context.Context) (err error)
}

// ParseSSM retrieves configuration from AWS SSM and populates the provided struct. It is a convenience function for
// Parse with a single SSM source named "ssm".
func ParseSSM(ctx context.Context, ssm *ssmpkg.Client, path string, cfg interface{}) (r Refresher, err error) {
	return Parse(ctx, cfg, false, SSMSource(ssm, path))
}

// ErrNoSource is returned when no sources are provided to the Parse function.
var ErrNoSource = errors.New("no sources provided")

// ErrSourceNotFound is returned when a specified source was not found in the list of sources.
var ErrSourceNotFound = errors.New("source not found")

// ErrGetParameters is returned when no parameters could not be fetched from a source.
var ErrGetParameters = errors.New("failed to get parameters")

// ErrBadFieldValue is returned when a value could not be set for a field.
var ErrBadFieldValue = errors.New("failed to set value for field")

// ErrBadDefaultFieldValue is returned when a specified default value could not be set for a field.
var ErrBadDefaultFieldValue = errors.New("failed to set default value for field")

// ErrParameterNotFound is returned when a parameter is not found in the source.
var ErrParameterNotFound = errors.New("parameter not found in source")

// Parse fetches configuration from the provided sources into the given struct.
// If a source is specified for a field, its value is queried only from that source.
// Otherwise, all sources are queried in order, with the last source's value taking precedence.
// Fields not tagged with `sky` are ignored unless `withUntagged` is true.
// Returns a Refresher for automatic configuration refresh.
//
// The configuration struct must have fields tagged with `sky` and the following tags. All tags are optional.
//   - default: sets the default value for the field.
//   - optional: marks the field as optional, suppressing errors if the field is not found in the source.
//   - flatten: flattens the field thereby ignoring the key of the outer struct.
//   - source: specifies the source for the field.
//   - refresh: sets the refresh duration for the field; duration must be in Go time.Duration format and greater than 0.
//   - id: sets the identifier for the field, used for update notifications.
func Parse(ctx context.Context, cfg interface{}, withUntagged bool, sources ...Source) (r Refresher, err error) {
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

	// Check if we have all the specified sources
	for _, field := range fields {
		if field.options.source == "" {
			continue
		}

		found := false
		for _, source := range sources {
			if source.ID() == field.options.source {
				found = true
				break
			}
		}

		if !found {
			err = fmt.Errorf("'%s' : %w", field.options.source, ErrSourceNotFound)
			return
		}
	}

	// First, process any default values for the fields
	for _, field := range fields {
		// If there is no default value, continue
		if field.options.defaultValue == "" {
			continue
		}

		// Process the default value for the field
		err = processFieldValue(true, field.options.defaultValue, field.structField)
		if err != nil {
			err = fmt.Errorf("%w of type %s: %w", ErrBadDefaultFieldValue, field.structField.Type(), err)
			return
		}
	}

	// Create an updater to handle refreshable fields.
	upd := &updater{}

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

		// Fetch the parameters from the source
		var values map[string]string
		values, err = source.Source(ctx, keys)
		if err != nil {
			err = fmt.Errorf("%w from source '%s' : %w", ErrGetParameters, source.ID(), err)
			return
		}

		// Process the fields based on the values obtained from the source
		for key, field := range fieldsMap {
			value, ok := values[key]

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

				var src string
				if field.options.source == "" && len(sources) > 1 {
					src = "(any)"
				} else {
					src = source.ID()
				}

				err = fmt.Errorf("%w - %s:%s", ErrParameterNotFound, src, key)
				return
			}

			// Process the field using the value obtained from the source
			if err = processFieldValue(false, value, field.structField); err != nil {
				err = fmt.Errorf("%w of type %s: %w", ErrBadFieldValue, field.structField.Type(), err)
				return
			}

			// If the field is refreshable, add it to the updater
			// NOTE that the field is added to the updater only if the value is successfully set the first time.
			if field.options.refresh != 0 {
				err = upd.add(field, key, source, value)
				if err != nil {
					return
				}
			}
		}
	}

	// If there are no refreshable fields, return an empty refresher
	if upd.empty() {
		r = noRefresh
		return
	}

	// Setup locking if the configuration struct is lockable
	upd.setupLock(cfg)

	// Return the updater as the refresher
	r = upd

	return
}
