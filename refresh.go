package skyconf

import (
	cfclock "code.cloudfoundry.org/clock"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sync"
	"time"
)

var ErrSourceNotRefreshable = errors.New("source is not refreshable")

var noRefresh = nilRefresh{
	updates: make(chan string),
	once:    &sync.Once{},
}

type nilRefresh struct {
	updates chan string
	once    *sync.Once
}

func (n nilRefresh) Refresh(ctx context.Context, _ func(err error)) <-chan string {
	go func() {
		<-ctx.Done()
		n.once.Do(func() {
			close(n.updates)
		})
	}()

	return n.updates
}

func (n nilRefresh) RefreshOnce(_ context.Context) (err error) {
	return
}

// ----------------------------------------------------------------------------

type nilLocker struct{}

func (n nilLocker) Lock() {}

func (n nilLocker) Unlock() {}

var nilLock nilLocker

// ----------------------------------------------------------------------------

type refreshedFieldSource struct {
	refreshedField
	Source
}

type refreshedField struct {
	field     fieldInfo
	key       string
	valueHash uint32 // CRC32 of the value
}

// refreshInfo is a struct that holds the information needed to refresh a field.
type refreshedFields struct {
	fields      []fieldInfo
	keys        []string
	valueHashes []uint32 // CRC32 of the values
}

// updater is a struct that holds the refresh information for the fields that have opted to be refreshed.
type updater struct {
	timings map[time.Duration]map[Source]*refreshedFields
	raw     []*refreshedFieldSource
	updates chan string
	clock   cfclock.Clock
	locker  sync.Locker
}

var ErrMissingKeyOnRefresh = errors.New("missing key on refresh")

func (u *updater) setupLock(i interface{}) {
	if i == nil {
		return
	}

	// Check if the interface is a locker
	if l, ok := i.(sync.Locker); ok {
		u.locker = l
	} else {
		u.locker = nilLock
	}
}

func (u *updater) Refresh(ctx context.Context, ef func(err error)) <-chan string {
	// Check if there are any fields to refresh
	if u.empty() {
		return noRefresh.Refresh(ctx, ef)
	}

	// If the error function is nil, set it to an empty function.
	if ef == nil {
		ef = func(err error) {}
	}

	// Process the raw list
	u.processRaw()

	// Initialise the clock
	if u.clock == nil {
		u.clock = cfclock.NewClock()
	}

	// When a timer ticks, send the ticker-channel to a channel
	tickChannel := make(chan (<-chan time.Time))

	// Set up the updates channel
	if u.updates == nil {
		u.updates = make(chan string)
	}

	// Map to keep track of the timings using the ticked channel
	timings := make(map[<-chan time.Time]map[Source]*refreshedFields, len(u.timings))

	// Keep track of the tickers to stop them when function returns
	tickers := make([]cfclock.Ticker, 0, len(u.timings))

	// Range over the timings and create a ticker for each duration
	for d, sfMap := range u.timings {
		ticker := u.clock.NewTicker(d)
		c := ticker.C()
		go func(c <-chan time.Time) {
			for range c {
				// Send the channel to tickChannel when the timer ticks
				tickChannel <- c
			}
		}(c)

		// Keep track of the ticker channel and the corresponding source-fields map in the timings map.
		timings[c] = sfMap

		// Add the ticker to the tickers slice
		tickers = append(tickers, ticker)
	}

	// Start the refresh goroutine.
	go func() {
		defer close(u.updates)
		defer close(tickChannel)

		// Stop tickers when this function returns
		defer func(tickers []cfclock.Ticker) {
			for _, t := range tickers {
				t.Stop()
			}
		}(tickers)

		// Loop to refresh fields
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
					continue
				}

				// Refresh the fields
				for source, fields := range rf {
					go u.refreshFieldsFromSource(ctx, source, fields, ef)
				}
			}
		}
	}()

	return u.updates
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

func (u *updater) Updates() <-chan string {
	return u.updates
}

// add adds a field to the updater, into a "raw" list, removing any duplicates.
func (u *updater) add(field fieldInfo, key string, source Source, value string) (err error) {
	// If the source is not refreshable, return an error
	if !source.Refreshable() {
		return fmt.Errorf("%w: %s", ErrSourceNotRefreshable, source.ID())
	}

	// CRC32 of the value
	crc := crc32.ChecksumIEEE([]byte(value))

	// Look through the raw list to see if the field is already added
	// If already added, replace the key and source
	for _, rfs := range u.raw {
		if rfs.field.structField == field.structField {
			rfs.key = key
			rfs.Source = source
			rfs.valueHash = crc
			return
		}
	}

	// Otherwise, add the field to the raw list
	u.raw = append(u.raw, &refreshedFieldSource{
		refreshedField: refreshedField{
			field:     field,
			key:       key,
			valueHash: crc,
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
		rf.valueHashes = append(rf.valueHashes, rfs.valueHash)
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
			// Check if the value has changed
			crc := crc32.ChecksumIEEE([]byte(val))
			if crc == rf.valueHashes[i] {
				continue
			}

			u.locker.Lock()
			err = processFieldValue(false, val, field.structField)
			u.locker.Unlock()

			// If there is no error, update the value hash and notify the updates channel
			if err == nil {
				rf.valueHashes[i] = crc

				// Set a timeout for the updates channel
				tc, cancel := context.WithTimeout(ctx, 500*time.Microsecond)

				// When sending updates, make sure we don't block the goroutine if there are no listeners
				select {
				case u.updates <- rf.fields[i].options.id:
				case <-tc.Done():
				}

				cancel()
			}
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
