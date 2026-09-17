package statusmap

import "testing"

func TestResolve(t *testing.T) {
	resolver := New(
		map[string]string{"Started": "Doing", "Completed": "Closed"},
		map[string]string{"started": "In Progress", "completed": "Done"},
		map[string]string{"Completed": "Done"},
	)
	tests := []struct {
		name       string
		stateName  string
		stateGroup string
		want       string
		resolution string
		wantOK     bool
	}{
		{name: "exact takes precedence", stateName: "Started", stateGroup: "started", want: "Doing", wantOK: true},
		{name: "resolution exact match", stateName: "Completed", stateGroup: "completed", want: "Closed", resolution: "Done", wantOK: true},
		{name: "group fallback", stateName: "Custom", stateGroup: "completed", want: "Done", wantOK: true},
		{name: "miss", stateName: "Custom", stateGroup: "cancelled"},
		{name: "name is case sensitive", stateName: "started", stateGroup: "started", want: "In Progress", wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, resolution, ok := resolver.Resolve(test.stateName, test.stateGroup)
			if got != test.want || resolution != test.resolution || ok != test.wantOK {
				t.Errorf("Resolve() = %q, %q, %t; want %q, %q, %t", got, resolution, ok, test.want, test.resolution, test.wantOK)
			}
		})
	}
}

func TestNewCopiesTables(t *testing.T) {
	exact := map[string]string{"Started": "Doing"}
	fallback := map[string]string{"started": "In Progress"}
	resolutions := map[string]string{"Started": "Done"}
	resolver := New(exact, fallback, resolutions)
	exact["Started"] = "Changed"
	fallback["started"] = "Changed"
	resolutions["Started"] = "Changed"

	if got, resolution, _ := resolver.Resolve("Started", "started"); got != "Doing" || resolution != "Done" {
		t.Errorf("Resolve() = %q after caller mutation", got)
	}
}
