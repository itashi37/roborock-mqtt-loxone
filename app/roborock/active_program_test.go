package roborock

import "testing"

func programStatus(key string) *PublishedStatus {
	return &PublishedStatus{Program: &key}
}

func TestResolveActiveProgramScene(t *testing.T) {
	program := ResolveActiveProgram(programStatus("scene:42"), []Scene{{ID: 42, Name: "Après les repas"}})
	if program.Kind != "scene" || program.SceneID != 42 || program.SceneName != "Après les repas" {
		t.Fatalf("program = %+v, want named scene", program)
	}
}

func TestResolveActiveProgramDoesNotGuessExternalScene(t *testing.T) {
	program := ResolveActiveProgram(programStatus("full"), []Scene{{ID: 42, Name: "Après les repas"}})
	if program.Kind != "full" || program.SceneID != 0 || program.SceneName != "" {
		t.Fatalf("program = %+v, want unidentified full clean", program)
	}
}

func TestResolveActiveProgramClearsWhenRunEnds(t *testing.T) {
	program := ResolveActiveProgram(&PublishedStatus{}, []Scene{{ID: 42, Name: "Après les repas"}})
	if program != (ActiveProgram{}) {
		t.Fatalf("program = %+v, want empty program", program)
	}
}
