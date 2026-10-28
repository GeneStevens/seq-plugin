package firstcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GeneStevens/seq/core"
	"github.com/GeneStevens/seq/testserver"
)

// TestBarleycrossChainThroughTheTools: the one smoke test that an agent can
// follow a quest chain with the tools the play guide names. It makes the calls
// the guide tells the agent to make (observe for offers and locked offers,
// talk_to to take one, quests for the step, go_to and use_door for the way,
// then waiting out the 45 s watch) and finishes the first two Barleycross
// quests. It is a scripted session, not a model: the calls are the guide's.
func TestBarleycrossChainThroughTheTools(t *testing.T) {
	binDir := buildPlugin(t, time.Now())
	s := startServer(t, testserver.Account{Name: "ann", Classed: map[string]string{"c1": "warrior"}})
	// A level 3 warrior in Hawthorn Square, by the greeter: the level the second
	// quest asks for, without the fights the chain does not need.
	s.Place("ann", "c1", core.Pos{X: 60000, Y: 45000})
	s.Give("c1", func(cs *core.CharacterSave) { cs.Stats.Level = 3 })
	c, _, _, stop := launchPlugin(t, s, binDir, "ann", "c1")
	defer stop()
	c.call("play_as", obj{"character": "c1"}, "Quest: First Steps")

	// The offer shows in observe, with the mark; look does not list offers.
	c.call("observe", obj{}, `quest: "Word to Barleycross", offered when you talk to it`, "New quest: Word to Barleycross")
	// One chain at a time: with First Steps running, the greeter's talk is its step. The
	// player has said to leave it, so the agent dismisses it.
	c.call("dismiss_quest", obj{}, "You dismiss First Steps")
	c.call("talk_to", obj{"npc": "the greeter"})
	c.call("quests", obj{}, "Word to Barleycross, step 1 of 1", "the way out is", "use_door")
	c.call("use_door", obj{"door": "east-gate"}, "Thornwick Downs")
	c.call("quests", obj{}, `talk_to "Tobias Hatch"`)

	// Hatch's other offers are locked, and say why.
	c.call("go_to", obj{"place": "Tobias Hatch"})
	c.call("observe", obj{}, `"Watch the Granary", not yet offered (Finish first: Word To Barleycross)`,
		`"Raiders' Spoil", not yet offered (Reach level 5)`)
	c.call("talk_to", obj{"npc": "Tobias Hatch"})
	c.call("quests", obj{}, "no quest chain")
	if got, _ := c.s.Character("c1"); got.Entity.Crowns != 8 || got.Entity.XP != 5 {
		t.Errorf("after Word to Barleycross: %d crowns and %d xp, want 8 and 5", got.Entity.Crowns, got.Entity.XP)
	}

	// The watch: take it, walk to the place, stay put out of a fight until the
	// progress is full, then tell Hatch.
	c.call("observe", obj{}, `"Watch the Granary", offered when you talk to it`)
	c.call("talk_to", obj{"npc": "Tobias Hatch"})
	c.call("quests", obj{}, "Watch the Granary, step 1 of 1", "stay near Barleycross for 45 s without fighting", "0 of 45 seconds")
	// quests says go_to "Barleycross", which the lane matches too (an error answer
	// that offers "nearest Barleycross"); the agent says that.
	c.call("go_to", obj{"place": "nearest Barleycross"})
	// The wait is the clock: nothing to do but stay put and look back. The step
	// is still on after 20 s, and done, by itself, after 45 (the chain's one step).
	c.s.Advance(20 * time.Second)
	c.call("quests", obj{}, "Watch the Granary", "stay near Barleycross for 45 s")
	if ch, _ := c.s.Character("c1"); ch.Entity.XP != 5 {
		t.Errorf("the watch paid before its 45 s: %d xp", ch.Entity.XP)
	}
	c.s.Advance(30 * time.Second)
	// The server settles the step on its own tick; under a loaded host that can
	// trail Advance, so wait for the pay before asking.
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if ch, _ := c.s.Character("c1"); ch.Entity.XP == 8 {
			break
		}
	}
	c.call("quests", obj{}, "no quest chain")
	if got, _ := c.s.Character("c1"); got.Entity.Crowns != 18 || got.Entity.XP != 8 {
		t.Errorf("after Watch the Granary: %d crowns and %d xp, want 18 and 8", got.Entity.Crowns, got.Entity.XP)
	}
	// The next quest is still locked by level, and the tool says so.
	c.call("observe", obj{}, `"Raiders' Spoil", not yet offered (Reach level 5)`)
}

// TestGuideHoldsNoLoreSecret: the guide is read by an agent helper, which is
// owed public lore only. No run of six words of any "## Secret" section of the
// pinned world's lore may appear in guide/PLAY.md.
func TestGuideHoldsNoLoreSecret(t *testing.T) {
	root := repoRoot(t)
	lore := filepath.Join(worldPacks(t, root, pins(t, root)), "thornwick", "lore")
	guide, err := os.ReadFile(filepath.Join(root, "guide", "PLAY.md"))
	if err != nil {
		t.Fatal(err)
	}
	words := func(s string) []string {
		return strings.Fields(strings.ToLower(regexp.MustCompile(`[^a-z0-9' ]+`).ReplaceAllString(strings.ToLower(s), " ")))
	}
	g := " " + strings.Join(words(string(guide)), " ") + " "
	n := 0
	err = filepath.Walk(lore, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, _ := os.ReadFile(p)
		in := false
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "## ") {
				in = strings.TrimSpace(l) == "## Secret"
				continue
			}
			if !in {
				continue
			}
			w := words(l)
			for i := 0; i+6 <= len(w); i++ {
				n++
				if run := strings.Join(w[i:i+6], " "); strings.Contains(g, " "+run+" ") {
					t.Errorf("guide/PLAY.md holds a run of a secret in %s: %q", filepath.Base(p), run)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("found no secret lore under %s: the check is not looking", lore)
	}
}
