package skyconf

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
	"time"
)

type mockParameterStore map[string]string

func (ps mockParameterStore) Source(params []string) (values map[string]string, err error) {
	if ps == nil {
		err = errInvalidSource
		return
	}

	values = make(map[string]string, len(params))
	for _, p := range params {
		// if p ends with "/an_error" return an error
		if strings.HasSuffix(p, "/an_error") {
			values = nil
			err = errMockSourceError
			return
		}

		value, ok := ps[p]
		if ok {
			values[p] = value
		}
	}

	return
}

type mockSource struct {
	ps   mockParameterStore
	path string
	id   string
}

var errInvalidSource = fmt.Errorf("invalid source")

var errMockSourceError = errors.New("mock source error")

func (m mockSource) Source(_ context.Context, params []string) (values map[string]string, err error) {
	return m.ps.Source(params)
}

func (m mockSource) ParameterName(parts []string) string {
	return makeParameterName(m.path, parts)
}

func (m mockSource) ID() string {
	if m.id != "" {
		return m.id
	}

	return "mock"
}

func TestParse(t *testing.T) {
	ps := mockParameterStore{
		"/path/config1-values/param1": "value1",
		"/path/config1-values/param2": "value2",
		"/path/config1-values/param3": "value3",

		"/path/error-config-values/int":      "BadValue",
		"/path/error-config-values/duration": "BadValue",

		"/multi-source/global/param1": "global-value1",
		"/multi-source/global/param2": "global-value2",
		"/multi-source/local/param2":  "local-value2",
	}

	type config1 struct {
		Param1 string `sky:"param1"`
		Param2 string `sky:"param2"`
		Param3 string `sky:"param3"`
		Param4 string `sky:"param4,optional"`
		Param5 string `sky:"param5,default:value5"`
	}

	type errorConfig struct {
		Int int `sky:"int"`
		/* Duration */ time.Duration
	}

	type badTags struct {
		Param1 string `sky:"param1,default:"`
	}

	type multiSourceConfig struct {
		Param1 string `sky:",source:source1"`
		Param2 string `sky:",source:source2"`
	}

	type layeredConfig struct {
		Param1 string `sky:"param1"`
		Param2 string `sky:"param2"`
	}

	tests := []struct {
		name    string
		cfg     interface{}
		sources []Source
		wantErr assert.ErrorAssertionFunc
		want    func(t *testing.T, cfg interface{})
	}{
		{
			name:    "no sources",
			cfg:     &config1{},
			sources: nil,
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrNoSource)
			},
		},
		{
			name: "error fetching parameters from source",
			cfg: &struct {
				Param1 string `sky:"an_error"`
			}{},
			sources: []Source{mockSource{
				ps:   ps,
				path: "/path/config1-values/",
			}},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errMockSourceError) &&
					assert.ErrorIs(t, err, ErrGetParameters)
			},
		},
		{
			name: "error setting field",
			cfg:  &errorConfig{},
			sources: []Source{mockSource{
				ps:   ps,
				path: "/path/error-config-values/",
			}},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrBadFieldValue)
			},
		},
		{
			name: "bad tags",
			cfg:  &badTags{},
			sources: []Source{mockSource{
				ps:   ps,
				path: "/path/bla/",
			}},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrBadTags)
			},
		},
		{
			name: "error setting default value",
			cfg: &struct {
				Param1 int `sky:",default:BadValue"`
			}{},
			sources: []Source{mockSource{
				ps: map[string]string{},
			}},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrBadDefaultFieldValue)
			},
		},
		{
			name: "no error if field is not found in source and is optional or has a default value",
			cfg: &struct {
				Param1 string `sky:"param1,optional"`
				Param2 string `sky:"param1,default:value2"`
			}{},
			sources: []Source{mockSource{
				ps: map[string]string{},
			}},
			wantErr: assert.NoError,
		},
		{
			name: "error if field is not found in source and is not optional or has no default value",
			cfg: &struct {
				Param1 string `sky:"param1"`
			}{},
			sources: []Source{mockSource{
				ps: map[string]string{},
			}},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrParameterNotFound)
			},
		},
		{
			name: "error if field is not found in any source",
			cfg: &struct {
				Param1 string `sky:"param1"`
			}{},
			sources: []Source{
				mockSource{
					ps: map[string]string{},
					id: "source1",
				},
				mockSource{
					ps: map[string]string{},
					id: "source2",
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrParameterNotFound)
			},
		},
		{
			name: "success with multiple sources",
			cfg:  &multiSourceConfig{},
			sources: []Source{
				mockSource{
					ps: map[string]string{
						"/path/source1-values/param1": "value1",
					},
					path: "/path/source1-values/",
					id:   "source1",
				},
				mockSource{
					ps: map[string]string{
						"/path/source2-values/param2": "value2",
					},
					path: "/path/source2-values/",
					id:   "source2",
				},
			},
			wantErr: assert.NoError,
			want: func(t *testing.T, cfg interface{}) {
				c := cfg.(*multiSourceConfig)
				assert.Equal(t, "value1", c.Param1)
				assert.Equal(t, "value2", c.Param2)
			},
		},
		{
			name: "success with layering sources",
			cfg:  &layeredConfig{},
			sources: []Source{
				mockSource{
					ps:   ps,
					path: "/multi-source/global/",
					id:   "global",
				},
				mockSource{
					ps:   ps,
					path: "/multi-source/local/",
					id:   "local",
				},
			},
			wantErr: assert.NoError,
			want: func(t *testing.T, cfg interface{}) {
				c := cfg.(*layeredConfig)
				assert.Equal(t, "global-value1", c.Param1)
				assert.Equal(t, "local-value2", c.Param2)
			},
		},
		{
			name: "success",
			cfg: &config1{
				Param4: "value4",
			},
			sources: []Source{mockSource{
				ps:   ps,
				path: "/path/config1-values/",
			}},
			wantErr: assert.NoError,
			want: func(t *testing.T, cfg interface{}) {
				c := cfg.(*config1)
				assert.Equal(t, "value1", c.Param1)
				assert.Equal(t, "value2", c.Param2)
				assert.Equal(t, "value3", c.Param3)
				assert.Equal(t, "value4", c.Param4)
				assert.Equal(t, "value5", c.Param5)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Parse(context.Background(), tt.cfg, true, tt.sources...)
			if tt.wantErr == nil {
				tt.wantErr = assert.NoError
			}

			if tt.wantErr(t, err) && err == nil && tt.want != nil {
				tt.want(t, tt.cfg)
			}
		})
	}
}
