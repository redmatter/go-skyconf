package skyconf

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

var ErrSourceNotRefreshable = errors.New("source is not refreshable")

var noRefresh = &nilRefresh{}

type nilRefresh struct{}

func (n *nilRefresh) Refresh(_ context.Context, _ func(err error)) (cancel func()) {
	return func() {
		// Do nothing
	}
}

func (n *nilRefresh) RefreshOnce(_ context.Context) (err error) {
	return
}

// ----------------------------------------------------------------------------

type refreshedFieldSource struct {
	refreshedField
	Source
}

type refreshedField struct {
	field fieldInfo
	key   string
}

// refreshInfo is a struct that holds the information needed to refresh a field.
type refreshedFields struct {
	fields []fieldInfo
	keys   []string
}

// updater is a struct that holds the refresh information for the fields that have opted to be refreshed.
type updater struct {
	timings map[time.Duration]map[Source]*refreshedFields
	raw     []*refreshedFieldSource
}

var ErrMissingKeyOnRefresh = errors.New("missing key on refresh")

func (u *updater) Refresh(ctx context.Context, ef func(err error)) (cancel func()) {
	// Check if there are any fields to refresh
	if u.empty() {
		return func() {} // Return an empty function that does nothing
	}

	// If the error function is nil, set it to an empty function.
	if ef == nil {
		ef = func(err error) {}
	}

	// Process the raw list
	u.processRaw()

	// Create a new context that will be used to cancel the refresh goroutine.
	ctx, cancel = context.WithCancel(ctx)

	// When a timer ticks, send the ticker-channel to a channel
	tickChannel := make(chan (<-chan time.Time))

	// Remap the timings using ticker channels
	timings := make(map[<-chan time.Time]map[Source]*refreshedFields, len(u.timings))

	// Keep track of the tickers to stop them when function returns
	tickers := make([]*time.Ticker, 0, len(u.timings))

	// Range over the timings and create a ticker for each duration
	for d, sfMap := range u.timings {
		ticker := time.NewTicker(d)
		go func() {
			for range ticker.C {
				// Send the ticker channel to tickChannel
				tickChannel <- ticker.C
			}
		}()

		// Keep track of the ticker channel and the corresponding source-fields map in the timings map.
		timings[ticker.C] = sfMap

		// Add the ticker to the tickers slice
		tickers = append(tickers, ticker)
	}

	// Start the refresh goroutine.
	go func() {
		// Stop tickers when this function returns
		defer func(tickers []*time.Ticker) {
			for _, t := range tickers {
				t.Stop()
			}
		}(tickers)

		defer close(tickChannel)

		for {
			select {
			// Check if the context has been cancelled
			case <-ctx.Done():
				return

			// Check if any timer has ticked
			case tc := <-tickChannel: // get the channel that ticked
				// Get the fields to refresh using the ticked channel
				rf, ok := timings[tc]
				if !ok {
					log.Println("ticked channel not found in timings")
					continue
				}

				log.Println("ticked channel found in timings")

				// Refresh the fields
				for source, fields := range rf {
					log.Println("refreshing fields from source:", source.ID())
					log.Println("fields:", fields.keys)
					go u.refreshFieldsFromSource(ctx, source, fields, ef)
				}
			}
		}
	}()

	return cancel
}

func (u *updater) RefreshOnce(ctx context.Context) (err error) {
	// Check if there are any fields to refresh
	if u.empty() {
		return
	}

	// Process the raw list
	u.processRaw()

	// Create a new context that will be used to cancel the refresh on first error.
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	// Refresh fields, irrespective of the timings, returning the first error that occurs
	for _, sourceFields := range u.timings {
		for source, rf := range sourceFields {
			u.refreshFieldsFromSource(ctx, source, rf, func(e error) {
				err = e
				cancel()
			})

			if err != nil {
				return
			}
		}
	}

	return
}

// add adds a field to the updater, into a "raw" list, removing any duplicates.
func (u *updater) add(field fieldInfo, key string, source Source) (err error) {
	// If the source is not refreshable, return an error
	if !source.Refreshable() {
		return fmt.Errorf("%w: %s", ErrSourceNotRefreshable, source.ID())
	}

	// Look through the raw list to see if the field is already added
	// If already added, replace the key and source
	for _, rfs := range u.raw {
		if rfs.field.structField == field.structField {
			rfs.key = key
			rfs.Source = source
			return
		}
	}

	// Otherwise, add the field to the raw list
	u.raw = append(u.raw, &refreshedFieldSource{
		refreshedField: refreshedField{
			field: field,
			key:   key,
		},
		Source: source,
	})

	return
}

func (u *updater) processRaw() {
	if u.timings == nil {
		u.timings = make(map[time.Duration]map[Source]*refreshedFields)
	}

	for _, rfs := range u.raw {
		timing, ok := u.timings[rfs.field.options.refresh]
		if !ok {
			u.timings[rfs.field.options.refresh] = make(map[Source]*refreshedFields)
			timing = u.timings[rfs.field.options.refresh]
		}

		rf := timing[rfs.Source]
		if rf == nil {
			rf = &refreshedFields{}
			timing[rfs.Source] = rf
		}

		// Add the field
		rf.fields = append(rf.fields, rfs.field)
		rf.keys = append(rf.keys, rfs.key)
	}

	// Clear the raw list
	u.raw = nil
}

func (u *updater) refreshFieldsFromSource(ctx context.Context, source Source, rf *refreshedFields, ef func(err error)) {
	var err error

	// handleErr will handle the error if err != nil and return true.
	handleErr := func() bool {
		if err != nil {
			ef(err)
			err = nil
			return true
		}

		return false
	}

	// Get the values for the keys
	var values map[string]string
	values, err = source.Source(ctx, rf.keys)
	if handleErr() {
		return
	}

	// Set the values for the fields
	for i, field := range rf.fields {
		if val, ok := values[rf.keys[i]]; ok {
			err = processFieldValue(false, val, field.structField)
		} else {
			err = fmt.Errorf("%w: %s", ErrMissingKeyOnRefresh, rf.keys[i])
		}

		handleErr()

		// Continue to the next field if the context has not been cancelled.
		if ctx.Err() != nil {
			return
		}
	}
}

func (u *updater) empty() bool {
	return len(u.raw) == 0
}
