package roborock

import (
	"strconv"
	"strings"
)

// ActiveProgram describes the cleaning program that the bridge can identify
// reliably. Scene details are present only when the run tracker associated a
// bridge-triggered scene command with the current cleaning run.
type ActiveProgram struct {
	Kind      string `json:"kind"`
	SceneID   int    `json:"scene_id"`
	SceneName string `json:"scene_name"`
}

func ResolveActiveProgram(status *PublishedStatus, scenes []Scene) ActiveProgram {
	if status == nil || status.Program == nil {
		return ActiveProgram{}
	}
	key := strings.TrimSpace(*status.Program)
	if key == "" {
		return ActiveProgram{}
	}

	if rawID, ok := strings.CutPrefix(key, "scene:"); ok {
		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			return ActiveProgram{Kind: "scene"}
		}
		program := ActiveProgram{Kind: "scene", SceneID: id}
		for _, scene := range scenes {
			if scene.ID == id {
				program.SceneName = scene.Name
				break
			}
		}
		return program
	}
	if strings.HasPrefix(key, "seg:") || key == "segment" {
		return ActiveProgram{Kind: "rooms"}
	}
	return ActiveProgram{Kind: key}
}
