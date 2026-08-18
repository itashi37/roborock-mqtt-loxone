package roborock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoxoneRoomOverrideStorePersistsAndDeletes(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLoxoneRoomOverrideStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]string{"23": "Room 23", "24": "Salon"}
	if err := store.Set("vacuum", 23, " Cuisine ", base); err != nil {
		t.Fatal(err)
	}
	if got := store.ForDevice("vacuum")["23"]; got != "Cuisine" {
		t.Fatalf("got %q, want Cuisine", got)
	}

	reloaded, err := NewLoxoneRoomOverrideStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ForDevice("vacuum")["23"]; got != "Cuisine" {
		t.Fatalf("persisted value got %q", got)
	}
	info, err := os.Stat(filepath.Join(dir, loxoneRoomOverridesFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("override file mode got %o, want 600", info.Mode().Perm())
	}
	if err := reloaded.Delete("vacuum", 23); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ForDevice("vacuum")) != 0 {
		t.Fatal("override was not deleted")
	}
}

func TestLoxoneRoomOverrideStoreRejectsAmbiguousNames(t *testing.T) {
	store, err := NewLoxoneRoomOverrideStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = store.Set("vacuum", 23, " salon ", map[string]string{"23": "Cuisine", "24": "Salon"})
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if len(store.ForDevice("vacuum")) != 0 {
		t.Fatal("rejected override was persisted")
	}
}

func TestLoxoneRoomOverrideStoreIgnoresNonCommandableConflicts(t *testing.T) {
	store, err := NewLoxoneRoomOverrideStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Segment 15 is historical and deliberately absent from baseNames, the
	// active commandable inventory supplied by get_room_mapping.
	store.devices["vacuum"] = map[string]string{"15": "Cuisine"}
	if err := store.Set("vacuum", 23, "Cuisine", map[string]string{"23": "Room 23", "24": "Salon"}); err != nil {
		t.Fatalf("historical override caused a false conflict: %v", err)
	}
}

func TestLoxoneDiagnosticStoreIsBounded(t *testing.T) {
	store := NewLoxoneDiagnosticStore(2)
	for i := int64(1); i <= 3; i++ {
		store.Record("vacuum", LoxoneActivity{Type: "event", TS: i, Event: "paused"})
	}
	snapshot := store.Snapshot("vacuum")
	if len(snapshot.Recent) != 2 || snapshot.Recent[0].TS != 2 || snapshot.Recent[1].TS != 3 {
		t.Fatalf("unexpected bounded history: %+v", snapshot.Recent)
	}
	command := LoxoneActivity{Type: "command", ID: "cmd-restored", State: "completed"}
	store.RestoreLastCommand("vacuum", command)
	snapshot = store.Snapshot("vacuum")
	if snapshot.LastCommand == nil || snapshot.LastCommand.ID != "cmd-restored" {
		t.Fatalf("last command not restored: %+v", snapshot.LastCommand)
	}
	if len(snapshot.Recent) != 2 {
		t.Fatal("restoring retained command changed current-session history")
	}
}
