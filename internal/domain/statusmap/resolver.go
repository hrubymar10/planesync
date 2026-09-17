package statusmap

// Resolver maps exact state names and fallback state groups to statuses.
type Resolver struct {
	exact         map[string]string
	groupFallback map[string]string
	resolutions   map[string]string
}

// New creates a resolver with defensive copies of both mapping tables.
func New(table, groupFallback, resolutionMap map[string]string) *Resolver {
	return &Resolver{
		exact:         clone(table),
		groupFallback: clone(groupFallback),
		resolutions:   clone(resolutionMap),
	}
}

// Resolve returns an exact state-name match or a state-group fallback.
func (r *Resolver) Resolve(stateName, stateGroup string) (string, string, bool) {
	if r == nil {
		return "", "", false
	}
	resolution := r.resolutions[stateName]
	if status, ok := r.exact[stateName]; ok {
		return status, resolution, true
	}
	status, ok := r.groupFallback[stateGroup]
	return status, resolution, ok
}

func clone(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
