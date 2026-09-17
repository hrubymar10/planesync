package linkmap

// Resolution describes how a target issue was resolved.
type Resolution int

const (
	// Mapped means the durable link map contained the target issue.
	Mapped Resolution = iota
	// Create means the link map did not contain a target issue.
	Create
)

// Hit is the result of looking up an existing target issue.
type Hit struct {
	Key string
	OK  bool
}

// Decide updates a mapped issue and creates an issue for a map miss.
func Decide(mapHit Hit) (string, Resolution) {
	if mapHit.OK {
		return mapHit.Key, Mapped
	}
	return "", Create
}
