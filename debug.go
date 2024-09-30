package skyconf

import (
	"fmt"
	"strings"
)

// anyFormatter is a formatter that can use any of the provided formatters.
type anyFormatter struct {
	formatters []Formatter
}

func (a anyFormatter) ParameterName(parts []string) string {
	var sb strings.Builder

	sb.WriteString("[ ")

	first := true
	for _, f := range a.formatters {
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

func String(cfg interface{}, withUntagged bool, formatters ...Formatter) (str string, err error) {
	// Print the config struct and its fields along with their tags and parameter names.
	// If withUntagged is true, untagged fields will also be printed.
	// If formatters are provided, the parameter names will be formatted using the provided formatters. Otherwise, a nilFormatter will be used.

	// Ensure we have a formatter.
	if len(formatters) == 0 {
		err = fmt.Errorf("no formatters provided")
		return
	}

	af := anyFormatter{formatters}

	// Make formatter func
	format := func(source string, parts []string, sb *strings.Builder) error {
		// Get the formatter for the source if specified.
		var f Formatter
		if source != "" {
			for _, f = range formatters {
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
			sb.WriteString("\n")
		}

		if err = format(field.options.source, field.nameParts, &sb); err != nil {
			return
		}
		sb.WriteString(" -> ")
		sb.WriteString(fmt.Sprintf("%+v", field.options))
		first = false
	}

	str = sb.String()
	return
}
