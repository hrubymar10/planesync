package linkmap

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name    string
		mapHit  Hit
		wantKey string
		want    Resolution
	}{
		{name: "update map hit", mapHit: Hit{Key: "mapped-key", OK: true}, wantKey: "mapped-key", want: Mapped},
		{name: "create on map miss", want: Create},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, resolution := Decide(test.mapHit)
			if key != test.wantKey || resolution != test.want {
				t.Errorf("Decide() = %q, %v; want %q, %v", key, resolution, test.wantKey, test.want)
			}
		})
	}
}
