package roborock

import (
	"reflect"
	"testing"
)

func TestRetainedTopicLedgerFindsRemovedRobotAndSlugTopics(t *testing.T) {
	dir := t.TempDir()
	ledger, err := NewRetainedTopicLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stale, err := ledger.Reconcile([]string{"home/old/status", "home/kept/status"}); err != nil || len(stale) != 0 {
		t.Fatalf("initial reconcile stale=%v err=%v", stale, err)
	}
	reloaded, err := NewRetainedTopicLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := reloaded.Reconcile([]string{"home/kept/status", "home/new/status"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stale, []string{"home/old/status"}) {
		t.Fatalf("stale topics=%v", stale)
	}
}
