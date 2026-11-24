package theme

import "testing"

func TestAgentColoursAreStable(t *testing.T) {
	Init(&Palette{Orange: RGB{245, 103, 40}, Cyan: RGB{68, 147, 146}, Magenta: RGB{151, 106, 221}})
	AssignAgents([]string{"claude", "pi", "hermes"})
	want := map[string]hue{"claude": orange, "pi": cyan, "hermes": magenta}
	for i := 0; i < 50; i++ { // unknown names must never reshuffle the others
		Agent("other"+string(rune('a'+i%26)), "x")
		for id, h := range want {
			if agentColor[id] != h {
				t.Fatalf("%s lost its colour: %v", id, agentColor[id])
			}
		}
	}
}
