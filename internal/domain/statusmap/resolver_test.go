package statusmap

import "testing"

func TestResolve(t *testing.T) {
	resolver := New(
		map[string]string{"Started": "Doing", "Completed": "Closed"},
		map[string]string{"started": "In Progress", "completed": "Done"},
	)
	tests := []struct {
		name       string
		stateName  string
		stateGroup string
		want       string
		wantOK     bool
	}{
		{name: "exact takes precedence", stateName: "Started", stateGroup: "started", want: "Doing", wantOK: true},
		{name: "group fallback", stateName: "Custom", stateGroup: "completed", want: "Done", wantOK: true},
		{name: "miss", stateName: "Custom", stateGroup: "cancelled"},
		{name: "name is case sensitive", stateName: "started", stateGroup: "started", want: "In Progress", wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := resolver.Resolve(test.stateName, test.stateGroup)
			if got != test.want || ok != test.wantOK {
				t.Errorf("Resolve() = %q, %t; want %q, %t", got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestNewCopiesTables(t *testing.T) {
	exact := map[string]string{"Started": "Doing"}
	fallback := map[string]string{"started": "In Progress"}
	resolver := New(exact, fallback)
	exact["Started"] = "Changed"
	fallback["started"] = "Changed"

	if got, _ := resolver.Resolve("Started", "started"); got != "Doing" {
		t.Errorf("Resolve() = %q after caller mutation", got)
	}
}
