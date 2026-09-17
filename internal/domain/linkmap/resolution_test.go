package linkmap

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name     string
		mapHit   Hit
		labelHit Hit
		wantKey  string
		want     Resolution
	}{
		{name: "map wins over label", mapHit: Hit{Key: "mapped-key", OK: true}, labelHit: Hit{Key: "labeled-key", OK: true}, wantKey: "mapped-key", want: Mapped},
		{name: "label recovers missing map", labelHit: Hit{Key: "labeled-key", OK: true}, wantKey: "labeled-key", want: Labeled},
		{name: "create when both miss", want: Create},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, resolution := Decide("source-item", test.mapHit, test.labelHit)
			if key != test.wantKey || resolution != test.want {
				t.Errorf("Decide() = %q, %v; want %q, %v", key, resolution, test.wantKey, test.want)
			}
		})
	}
}
