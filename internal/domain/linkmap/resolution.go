package linkmap

// Resolution describes how a target issue was resolved.
type Resolution int

const (
	// Mapped means the durable link map contained the target issue.
	Mapped Resolution = iota
	// Labeled means a target issue was recovered through its idempotency label.
	Labeled
	// Create means no existing target issue was found.
	Create
)

// Hit is the result of looking up an existing target issue.
type Hit struct {
	Key string
	OK  bool
}

// Decide applies map, then label, then create precedence.
func Decide(_ string, mapHit, labelHit Hit) (string, Resolution) {
	if mapHit.OK {
		return mapHit.Key, Mapped
	}
	if labelHit.OK {
		return labelHit.Key, Labeled
	}
	return "", Create
}
