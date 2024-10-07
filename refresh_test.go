package skyconf

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestParseAndRefresh(t *testing.T) {
	parameterStore := func() mockParameterStore {
		return mockParameterStore{
			"/path/global/param1": "global-value1",
			"/path/global/param2": "global-value2",

			"/path/region1/param2": "region1-value2",
		}
	}

	type config struct {
		Param1 string `sky:",source:global,refresh:1s"`
		Param2 string `sky:",refresh:1s"`
	}

	tests := []struct {
		name    string
		cfg     interface{}
		sources []Source
		wantErr assert.ErrorAssertionFunc
		want    func(t *testing.T, sources []Source, cfg interface{}, r Refresher)
	}{
		{
			name: "refresh once",
			cfg:  &config{},
			sources: []Source{
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/global/",
					id:          "global",
					refreshable: true,
				},
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/region1/",
					id:          "regional",
					refreshable: true,
				},
			},
			wantErr: assert.NoError,
			want: func(t *testing.T, sources []Source, cfg interface{}, r Refresher) {
				c := cfg.(*config)
				assert.Equal(t, "global-value1", c.Param1)
				assert.Equal(t, "region1-value2", c.Param2)

				// update the values
				sources[0].(*mockSource).set("/path/global/param1", "new-global-value1")
				sources[0].(*mockSource).set("/path/global/param2", "new-global-value2")

				sources[1].(*mockSource).set("/path/region1/param2", "new-region1-value2")

				// setup refresh
				err := r.RefreshOnce(context.Background())
				if assert.NoError(t, err) {
					// Check the values after refresh
					assert.Equal(t, "new-global-value1", c.Param1)
					assert.Equal(t, "new-region1-value2", c.Param2)
				}
			},
		},
		{
			name: "refresh continuously",
			cfg:  &config{},
			sources: []Source{
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/global/",
					id:          "global",
					refreshable: true,
				},
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/region1/",
					id:          "regional",
					refreshable: true,
				},
			},
			wantErr: assert.NoError,
			want: func(t *testing.T, sources []Source, cfg interface{}, r Refresher) {
				c := cfg.(*config)
				assert.Equal(t, "global-value1", c.Param1)
				assert.Equal(t, "region1-value2", c.Param2)

				// Update values at different intervals
				go func() {
					time.Sleep(900 * time.Millisecond)
					sources[0].(*mockSource).set("/path/global/param1", "new1-global-value1")
					sources[0].(*mockSource).set("/path/global/param2", "new1-global-value2")

					sources[1].(*mockSource).set("/path/region1/param2", "new1-region1-value2")
				}()
				go func() {
					time.Sleep(time.Second + 900*time.Millisecond)
					sources[0].(*mockSource).set("/path/global/param1", "new2-global-value1")
					sources[0].(*mockSource).set("/path/global/param2", "new2-global-value2")

					sources[1].(*mockSource).set("/path/region1/param2", "new2-region1-value2")
				}()
				go func() {
					time.Sleep(2*time.Second + 900*time.Millisecond)
					sources[0].(*mockSource).set("/path/global/param1", "new3-global-value1")
					sources[0].(*mockSource).set("/path/global/param2", "new3-global-value2")

					sources[1].(*mockSource).set("/path/region1/param2", "new3-region1-value2")
				}()

				// setup refresh
				cancel := r.Refresh(context.Background(), nil)
				// Stop refreshing when the test is done
				defer cancel()

				// Check values at different intervals; at 1, 2, and 3 seconds (plus some buffer)
				time.Sleep(time.Second + 100*time.Millisecond)
				assert.Equal(t, "new1-global-value1", c.Param1)
				assert.Equal(t, "new1-region1-value2", c.Param2)

				time.Sleep(time.Second + 100*time.Millisecond)
				assert.Equal(t, "new2-global-value1", c.Param1)
				assert.Equal(t, "new2-region1-value2", c.Param2)

				time.Sleep(time.Second + 100*time.Millisecond)
				assert.Equal(t, "new3-global-value1", c.Param1)
				assert.Equal(t, "new3-region1-value2", c.Param2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Parse(context.Background(), tt.cfg, true, tt.sources...)
			if tt.wantErr == nil {
				tt.wantErr = assert.NoError
			}

			if tt.wantErr(t, err) && err == nil && tt.want != nil {
				tt.want(t, tt.sources, tt.cfg, r)
			}
		})
	}
}
