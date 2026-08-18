package templates

import "testing"

func TestRegistryFailsClosedWithoutValidatedFixtures(t *testing.T) {
	status := StatusForCurrentBuild()
	if status.NativeGeneration || status.FormatVerified || len(status.RequiredSamples) != 5 {
		t.Fatalf("unsafe template status: %+v", status)
	}
}
