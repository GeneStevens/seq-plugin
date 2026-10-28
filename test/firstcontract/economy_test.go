package firstcontract

import (
	"strings"
	"testing"
	"time"

	"github.com/GeneStevens/seq/core"
	"github.com/GeneStevens/seq/testserver"
)

// TestDraughtVaultAndPuntThroughTheTools: the one smoke test that an agent can
// do what the guide's "Spending crowns" section says with the tools it names:
// walk to a seller with go_to, buy a draught with act, use it, buy a vault tier
// at the banker, and take Nell's punt. It is a scripted session, not a model.
func TestDraughtVaultAndPuntThroughTheTools(t *testing.T) {
	binDir := buildPlugin(t, time.Now())
	s := startServer(t, testserver.Account{Name: "ann", Classed: map[string]string{"c1": "warrior"}},
		testserver.Account{Name: "bob", Home: "thornwick-downs", Classed: map[string]string{"c2": "warrior"}})
	s.Place("ann", "c1", core.Pos{X: 60000, Y: 45000})
	s.Give("c1", func(cs *core.CharacterSave) { cs.Crowns = 200; cs.HP = 5 })
	c, _, _, stop := launchPlugin(t, s, binDir, "ann", "c1")
	defer stop()
	c.call("play_as", obj{"character": "c1"})

	// The binder sells the draughts; the answer to a use names the heal.
	c.call("go_to", obj{"place": "Isolde Fenn"})
	c.call("act", obj{"intent": "buy", "npc": "binder", "item": "small-hearth-draught", "count": 1})
	c.call("observe", obj{}, "small-hearth-draught", "use_item: heals 40")
	c.call("act", obj{"intent": "use_item", "item": "small-hearth-draught"})
	if got, _ := c.s.Character("c1"); got.Entity.Crowns != 185 || got.Entity.HP < 40 {
		t.Errorf("after one draught: %d crowns and %d hp, want 185 and at least 40", got.Entity.Crowns, got.Entity.HP)
	}

	// The banker's next vault tier: 20 slots for 100 crowns.
	c.call("go_to", obj{"place": "Odalys Penn"})
	c.call("observe", obj{}, "bank_expand")
	c.call("act", obj{"intent": "bank_expand", "npc": "banker"})
	if got, _ := c.s.Character("c1"); got.Entity.Crowns != 85 || got.Entity.BankTiers != 1 {
		t.Errorf("after the vault tier: %d crowns and %d tiers, want 85 and 1", got.Entity.Crowns, got.Entity.BankTiers)
	}

	// The punt: ten crowns across the Millrace to Eelmere. A second character,
	// homed in the Downs, stands at the landing, so the test does not walk the Downs past brigands.
	s.Place("bob", "c2", core.Pos{X: 199000, Y: 308000})
	s.Give("c2", func(cs *core.CharacterSave) { cs.Crowns = 25 })
	d, _, _, stop2 := launchPlugin(t, s, binDir, "bob", "c2")
	defer stop2()
	d.call("play_as", obj{"character": "c2"})
	d.call("observe", obj{}, "nells-punt", "to-eelmere")
	d.call("act", obj{"intent": "travel", "npc": "nells-punt", "route": "to-eelmere"})
	// SEQ-193: a ferry trip is told as travel, never as a death's respawn.
	if ev := d.call("recent_events", obj{}, "traveled: You sail with Nell's Punt to Eelmere"); strings.Contains(ev, "respawned") {
		t.Errorf("the punt was told as a respawn:\n%s", ev)
	}
	if got, _ := d.s.Character("c2"); got.Entity.Crowns != 15 || got.Entity.Pos.Y < 400000 {
		t.Errorf("after the punt: %d crowns at y=%d, want 15 at Eelmere", got.Entity.Crowns, got.Entity.Pos.Y)
	}
}
