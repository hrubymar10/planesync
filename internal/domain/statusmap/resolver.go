package statusmap

// Resolver maps exact state names and fallback state groups to statuses.
type Resolver struct {
	exact         map[string]string
	groupFallback map[string]string
}

// New creates a resolver with defensive copies of both mapping tables.
func New(table, groupFallback map[string]string) *Resolver {
	return &Resolver{
		exact:         clone(table),
		groupFallback: clone(groupFallback),
	}
}

// Resolve returns an exact state-name match or a state-group fallback.
func (r *Resolver) Resolve(stateName, stateGroup string) (string, bool) {
	if r == nil {
		return "", false
	}
	if status, ok := r.exact[stateName]; ok {
		return status, true
	}
	status, ok := r.groupFallback[stateGroup]
	return status, ok
}

func clone(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
