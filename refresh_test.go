package skyconf

import (
	"code.cloudfoundry.org/clock/fakeclock"
	"context"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

type config struct {
	Param1 string `sky:",source:global,refresh:1s"`
	Param2 string `sky:",refresh:1s"`
	m      sync.Mutex
}

func (c *config) Lock() {
	c.m.Lock()
}

func (c *config) Unlock() {
	c.m.Unlock()
}

func TestParseAndRefreshUsingMockClock(t *testing.T) {
	parameterStore := func() mockParameterStore {
		return mockParameterStore{
			"/path/global/param1": "global-value1",
			"/path/global/param2": "global-value2",

			"/path/region1/param2": "region1-value2",
		}
	}

	type assertUpdatesInput struct {
		id       string
		expected string
		field    *string
	}

	assertUpdates := func(t *testing.T, updates <-chan string, expected ...assertUpdatesInput) bool {
		// Make a map of expected updates to mark them as received
		expectedMap := make(map[int]*bool, len(expected))

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Collect as many updates as expected
		for range expected {
			var u string
			select {
			case <-ctx.Done():
				assert.Fail(t, "timed out waiting for updates")
				return false
			case u = <-updates:
			}

			// find the expected update using the ID
			for i, e := range expected {
				// if already received, skip
				if _, done := expectedMap[i]; done {
					continue
				}

				if e.id == u {
					a := assert.Equal(t, e.expected, *(e.field))
					expectedMap[i] = &a
					break
				}
			}
		}

		// Ensure all expected updates were received
		for i, v := range expectedMap {
			if v == nil {
				assert.Failf(t, "expected update not received", "expected: %v", expected[i])
				return false
			}

			if !assert.Truef(t, v != nil, "expected update not received; %v", expected[i]) {
				return false
			}
		}

		return true
	}

	tests := []struct {
		name    string
		cfg     interface{}
		sources []Source
		wantErr assert.ErrorAssertionFunc
		want    func(t *testing.T, sources []Source, cfg interface{}, r Refresher, clock *fakeclock.FakeClock)
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
			want: func(t *testing.T, sources []Source, cfg interface{}, r Refresher, clock *fakeclock.FakeClock) {
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
			want: func(t *testing.T, sources []Source, cfg interface{}, r Refresher, clock *fakeclock.FakeClock) {
				c := cfg.(*config)
				assert.Equal(t, "global-value1", c.Param1)
				assert.Equal(t, "region1-value2", c.Param2)

				// setup refresh
				ctx, cancel := context.WithCancel(context.Background())
				updates := r.Refresh(ctx, func(err error) {
					if assert.NoError(t, err) {
						return
					}

					cancel()
				})

				allUpdates := make(chan string, 1000)
				go func() {
					for u := range updates {
						allUpdates <- u
					}
				}()

				// Stop refreshing when the test is done
				defer cancel()

				// Timeout the test after 5 seconds
				go func() {
					select {
					case <-time.After(5 * time.Second):
						assert.Failf(t, "test timed out", "")
						cancel()
					case <-ctx.Done():
					}
				}()

				// Update the values in the sources
				sources[0].(*mockSource).set("/path/global/param1", "new1-global-value1")
				sources[0].(*mockSource).set("/path/global/param2", "new1-global-value2")
				sources[1].(*mockSource).set("/path/region1/param2", "new1-region1-value2")

				// Advance the clock to trigger the refresh
				clock.Increment(time.Second + 1*time.Millisecond)
				if !assertUpdates(t, allUpdates,
					assertUpdatesInput{"Param1", "new1-global-value1", &c.Param1},
					assertUpdatesInput{"Param2", "new1-region1-value2", &c.Param2},
				) {
					return
				}

				// Update the values in the sources
				sources[0].(*mockSource).set("/path/global/param1", "new2-global-value1")
				sources[0].(*mockSource).set("/path/global/param2", "new2-global-value2")
				sources[1].(*mockSource).set("/path/region1/param2", "new2-region1-value2")

				// Advance the clock to trigger the refresh
				clock.Increment(time.Second + 1*time.Millisecond)
				if !assertUpdates(t, allUpdates,
					assertUpdatesInput{"Param1", "new2-global-value1", &c.Param1},
					assertUpdatesInput{"Param2", "new2-region1-value2", &c.Param2},
				) {
					return
				}

				// Update the values in the sources
				sources[0].(*mockSource).set("/path/global/param1", "new3-global-value1")
				sources[0].(*mockSource).set("/path/global/param2", "new3-global-value2")
				sources[1].(*mockSource).set("/path/region1/param2", "new3-region1-value2")

				// Advance the clock to trigger the refresh
				clock.Increment(time.Second + 1*time.Millisecond)
				if !assertUpdates(t, allUpdates,
					assertUpdatesInput{"Param1", "new3-global-value1", &c.Param1},
					assertUpdatesInput{"Param2", "new3-region1-value2", &c.Param2},
				) {
					return
				}

				cancel()
			},
		},
		{
			name: "refresh continuously with no refreshable sources",
			cfg:  &config{},
			sources: []Source{
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/global/",
					id:          "global",
					refreshable: false,
				},
				&mockSource{
					ps:          parameterStore(),
					path:        "/path/region1/",
					id:          "regional",
					refreshable: false,
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrSourceNotRefreshable)
			},
		},
	}

	testTime := time.Date(2021, 1, 1, 1, 1, 1, 1, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := fakeclock.NewFakeClock(testTime)
			r, err := Parse(context.Background(), tt.cfg, true, tt.sources...)
			if tt.wantErr == nil {
				tt.wantErr = assert.NoError
			}

			if tt.wantErr(t, err) && err == nil && tt.want != nil {
				// If updater, set the mock clock
				if r != nil {
					upd, ok := r.(*updater)
					if ok {
						upd.clock = clock
					}
					tt.want(t, tt.sources, tt.cfg, upd, clock)
				} else {
					tt.want(t, tt.sources, tt.cfg, r, clock)
				}

			}
		})
	}
}
