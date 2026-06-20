package chunk

// Flusher persists in-memory or cached chunk data to durable local storage.
type Flusher interface {
	Flush() (int, error)
}

// FlushStore syncs dirty chunks and fsyncs disk-backed stores when supported.
func FlushStore(s Store) (int, error) {
	if f, ok := s.(Flusher); ok {
		return f.Flush()
	}
	return 0, nil
}
