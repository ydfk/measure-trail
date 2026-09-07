package legacy

import "testing"

func TestWeightsAreConsistentAllowsLegacyRounding(t *testing.T) {
	for _, test := range []struct {
		name             string
		weightCentigrams int
		jinCentigrams    int
		want             bool
	}{
		{name: "exact", weightCentigrams: 7182, jinCentigrams: 14364, want: true},
		{name: "rounds up one centigram", weightCentigrams: 7182, jinCentigrams: 14365, want: true},
		{name: "rounds down one centigram", weightCentigrams: 7183, jinCentigrams: 14365, want: true},
		{name: "rejects material mismatch", weightCentigrams: 7182, jinCentigrams: 14366, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := weightsAreConsistent(test.weightCentigrams, test.jinCentigrams); got != test.want {
				t.Fatalf("weightsAreConsistent() = %v, want %v", got, test.want)
			}
		})
	}
}
