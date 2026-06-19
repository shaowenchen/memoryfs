package chunk

// Store is the local chunk storage backend on a node.
type Store interface {
	Get(id string) ([]byte, bool)
	Put(id string, data []byte) error
	Delete(id string) error
	List() []string
	Count() int
}
