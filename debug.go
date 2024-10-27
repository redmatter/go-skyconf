package skyconf

import (
	"context"
	"fmt"
	"strings"
)

type anyFormatter struct {
	sources []Source
}

func (a anyFormatter) Source(_ context.Context, _ []string) (values map[string]string, err error) {
	panic("not implemented")
}

func (a anyFormatter) ParameterName(parts []string) string {
	var sb strings.Builder

	sb.WriteString("[ ")

	first := true
	for _, f := range a.sources {
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(f.ID() + ":" + f.ParameterName(parts))
		first = false
	}
	sb.WriteString(" ]")

	return sb.String()
}

func (a anyFormatter) ID() string {
	return "anyOf"
}

func (a anyFormatter) Refreshable() bool {
	return true
}

// String returns a string representation of the provided configuration struct, describing source and parameter name for
// each field.
func String(cfg interface{}, withUntagged bool, sources ...Source) (str string, err error) {
	// Ensure we have a formatter.
	if len(sources) == 0 {
		err = fmt.Errorf("no sources provided")
		return
	}

	af := anyFormatter{sources}

	// Make formatter func
	format := func(source string, parts []string, sb *strings.Builder) error {
		// Get the formatter for the source if specified.
		var f Source
		if source != "" {
			for _, f = range sources {
				if f.ID() == source {
					break
				}
			}

			// If we didn't find a formatter, return an error.
			if f == nil {
				return fmt.Errorf("no formatter found for source %s", source)
			}
		} else {
			f = af
		}

		sb.WriteString(f.ID() + ":" + f.ParameterName(parts))
		return nil
	}

	var fields []fieldInfo
	fields, err = extractFields(withUntagged, nil, cfg, fieldOptions{})
	if err != nil {
		return
	}

	var sb strings.Builder
	first := true
	for _, field := range fields {
		if !first {
			sb.Write([]byte{'\n'})
		}

		if err = format(field.options.source, field.nameParts, &sb); err != nil {
			return
		}
		sb.WriteString(" -> ")
		sb.WriteString(field.options.String())
		first = false
	}

	str = sb.String()
	return
}
