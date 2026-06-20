package mountlog

import (
	"log"
	"time"
)

// Verbose enables debug-level mount/client I/O logs (-v).
var Verbose bool

// SetVerbose toggles debug logging.
func SetVerbose(v bool) { Verbose = v }

// Infof logs always.
func Infof(format string, args ...any) {
	log.Printf(format, args...)
}

// Debugf logs when Verbose is enabled.
func Debugf(format string, args ...any) {
	if Verbose {
		log.Printf("debug "+format, args...)
	}
}

// Warnf logs warnings (always).
func Warnf(format string, args ...any) {
	log.Printf("warn "+format, args...)
}

// Errorf logs errors (always).
func Errorf(format string, args ...any) {
	log.Printf("error "+format, args...)
}

// Timed runs fn and logs duration on verbose or failure.
func Timed(label string, fn func() error) error {
	start := time.Now()
	err := fn()
	d := time.Since(start)
	if err != nil {
		Errorf("%s failed after %s: %v", label, d, err)
		return err
	}
	Debugf("%s ok in %s", label, d)
	return nil
}
