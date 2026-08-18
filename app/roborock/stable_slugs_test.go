package roborock

import "testing"

func TestStableSlugStoreSurvivesRenameAndOrderChanges(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStableSlugStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Resolve([]DeviceInfo{{DID: "did-a", Name: "Salon"}, {DID: "did-b", Name: "Salon"}})
	if err != nil {
		t.Fatal(err)
	}
	if first[0] != "salon" || first[1] != "salon-2" {
		t.Fatalf("unexpected initial slugs: %v", first)
	}

	reloaded, err := NewStableSlugStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reloaded.Resolve([]DeviceInfo{{DID: "did-b", Name: "Étage"}, {DID: "did-a", Name: "Cuisine"}})
	if err != nil {
		t.Fatal(err)
	}
	if second[0] != "salon-2" || second[1] != "salon" {
		t.Fatalf("persistent slugs changed after rename/reorder: %v", second)
	}
}

func TestDeviceManagerUsesPersistentSlugs(t *testing.T) {
	dir := t.TempDir()
	first := NewDeviceManager(&LoginData{}, []DeviceInfo{{DID: "did-a", Name: "Robot"}}, nil, dir)
	if got := first.GetDevices()[0].Slug; got != "robot" {
		t.Fatalf("initial slug = %q", got)
	}
	second := NewDeviceManager(&LoginData{}, []DeviceInfo{{DID: "did-a", Name: "Renamed"}}, nil, dir)
	if got := second.GetDevices()[0].Slug; got != "robot" {
		t.Fatalf("slug changed after rename: %q", got)
	}
}
