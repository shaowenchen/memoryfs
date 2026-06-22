package chunk

// LocalRef returns chunk bytes without copying when the store supports it.
func LocalRef(s Store, id string) ([]byte, bool) {
	type refStore interface {
		chunkRef(string) ([]byte, bool)
	}
	if rs, ok := s.(refStore); ok {
		return rs.chunkRef(id)
	}
	return s.Get(id)
}
