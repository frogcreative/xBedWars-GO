package game

import (
	"fmt"
	"strconv"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/cfg"
	"github.com/frogcreative/bedwars/internal/util"
)

// Gen is a resource generator. Iron and gold belong to a team; diamond and emerald are shared.
// Only shared generators have a floating label.
type Gen struct {
	Kind  string
	Pos   util.Vec
	Level int
	Timer int

	label *world.EntityHandle
}

// genInterval returns the seconds between drops.
func genInterval(kind string, level int) int {
	switch kind {
	case "iron":
		return cfg.Int("game.generator.iron-interval", 2)
	case "gold":
		return cfg.Int("game.generator.gold-interval", 6)
	case "diamond":
		return map[int]int{1: 30, 2: 20, 3: 15}[level]
	case "emerald":
		return map[int]int{1: 70, 2: 50, 3: 30}[level]
	case "emeraldTeam":
		return 70
	}
	return 0
}

// genCap returns the maximum number of items that may lie around the generator.
func genCap(kind string, level int) int {
	switch kind {
	case "iron":
		return 32
	case "gold":
		return 7
	case "emeraldTeam":
		return 4
	}
	switch level {
	case 1:
		return 4
	case 2:
		return 6
	case 3:
		return 8
	}
	return 4
}

// genAmount returns the items produced per drop for a forge level.
func genAmount(level int) int {
	switch {
	case level < 2:
		return 1
	case level < 5:
		return 2
	}
	return 3
}

// genStack returns the item produced by a generator.
func genStack(kind string, n int) item.Stack {
	switch kind {
	case "iron":
		return item.NewStack(item.IronIngot{}, n)
	case "gold":
		return item.NewStack(item.GoldIngot{}, n)
	case "diamond":
		return item.NewStack(item.Diamond{}, n)
	}
	return item.NewStack(item.Emerald{}, n)
}

// vec3 converts a util.Vec to a Dragonfly vector.
func vec3(v util.Vec) mgl64.Vec3 { return mgl64.Vec3{v.X, v.Y, v.Z} }

// initGenerator creates every team and shared generator for a new match.
func (g *Game) initGenerator() {
	g.Gens = nil
	for _, team := range g.Data.TeamName {
		tg := g.Data.TeamGenerator[team]
		for _, kind := range []string{"iron", "gold", "emeraldTeam"} {
			key := kind
			if kind == "emeraldTeam" {
				key = "emerald"
			}
			s, ok := tg[key]
			if !ok {
				continue
			}
			g.Gens = append(g.Gens, &Gen{Kind: kind, Pos: util.MustVec(s).Add(0.5, 0, 0.5), Level: 1, Timer: genInterval(kind, 1)})
		}
	}
	for _, kind := range []string{"diamond", "emerald"} {
		for _, s := range g.Data.Generator[kind] {
			ge := &Gen{Kind: kind, Pos: util.MustVec(s).Add(0.5, 1, 0.5), Level: 1, Timer: genInterval(kind, 1)}
			ge.label = entity.NewText(genLabel(ge), vec3(ge.Pos.Add(0, 1.2, 0)))
			g.Gens = append(g.Gens, ge)
			h := ge.label
			g.World.Exec(func(tx *world.Tx) { tx.AddEntity(h) })
		}
	}
}

// roman formats 1-5 as a roman numeral.
func roman(n int) string {
	return map[int]string{1: "I", 2: "II", 3: "III", 4: "IV", 5: "V"}[n]
}

// genLabel builds the floating label of a shared generator.
func genLabel(ge *Gen) string {
	name := "§bDiamond"
	if ge.Kind == "emerald" {
		name = "§aEmerald"
	}
	return fmt.Sprintf("§eTier §c%s\n%s\n\n§eSpawns in §c%d §eseconds!", roman(ge.Level), name, ge.Timer)
}

// tickGens advances every generator by one second. Dropping runs on the world goroutine.
func (g *Game) tickGens() {
	w := g.World
	for _, ge := range g.Gens {
		if ge.Kind == "emeraldTeam" && ge.Level < 3 {
			continue
		}
		ge.Timer--
		shared := ge.Kind == "diamond" || ge.Kind == "emerald"
		if shared && ge.label != nil {
			text := genLabel(ge)
			h := ge.label
			go h.ExecWorld(func(_ *world.Tx, e world.Entity) {
				if t, ok := e.(*entity.Ent); ok {
					t.SetNameTag(text)
				}
			})
		}
		if ge.Timer > 0 {
			continue
		}
		ge.Timer = genInterval(ge.Kind, ge.Level)
		kind, level, pos := ge.Kind, ge.Level, ge.Pos
		w.Exec(func(tx *world.Tx) { produce(tx, kind, level, pos) })
	}
}

// produce drops (or hands out) the generator output. Team generators give items directly to
// players standing on them when two or more are present; shared generators always drop. It runs on
// the world goroutine.
func produce(tx *world.Tx, kind string, level int, pos util.Vec) {
	capN := genCap(kind, level)
	near := countItems(tx, pos, 3, kind)
	capped := near >= capN
	centre := vec3(pos)

	drop := func(n int, at mgl64.Vec3, vel mgl64.Vec3) {
		tx.AddEntity(entity.NewItem(world.EntitySpawnOpts{Position: at, Velocity: vel}, genStack(kind, n)))
	}

	if kind == "diamond" || kind == "emerald" {
		if !capped {
			drop(1, centre.Add(mgl64.Vec3{0, -0.5, 0}), mgl64.Vec3{0, -0.3, 0})
		}
		return
	}

	amount := genAmount(level)
	if kind == "emeraldTeam" {
		amount = 1
	}
	players := playersNear(tx, pos, 1)
	switch {
	case len(players) == 0 && !capped:
		drop(amount, centre.Add(mgl64.Vec3{0, 0.5, 0}), mgl64.Vec3{0, -1, 0})
	case len(players) <= 1 && !capped:
		drop(amount, centre.Add(mgl64.Vec3{0, 0.5, 0}), mgl64.Vec3{0, -1, 0})
	default:
		for _, p := range players {
			if _, err := p.Inventory().AddItem(genStack(kind, amount)); err != nil {
				if !capped {
					drop(amount, centre.Add(mgl64.Vec3{0, 0.5, 0}), mgl64.Vec3{0, -1, 0})
				}
			}
		}
	}
}

// box returns the cube of radius r around pos.
func box(pos util.Vec, r float64) cube.BBox {
	return cube.Box(pos.X-r, pos.Y-r, pos.Z-r, pos.X+r, pos.Y+r, pos.Z+r)
}

// countItems counts the dropped items of kind around a generator.
// Each dropped stack counts once, regardless of how many items it contains.
func countItems(tx *world.Tx, pos util.Vec, r float64, kind string) int {
	n := 0
	for e := range tx.EntitiesWithin(box(pos, r)) {
		ent, ok := e.(*entity.Ent)
		if !ok {
			continue
		}
		ib, ok := ent.Behaviour().(*entity.ItemBehaviour)
		if !ok {
			continue
		}
		st := ib.Item()
		switch kind {
		case "iron":
			if _, ok := st.Item().(item.IronIngot); ok {
				n++
			}
		case "gold":
			if _, ok := st.Item().(item.GoldIngot); ok {
				n++
			}
		case "diamond":
			if _, ok := st.Item().(item.Diamond); ok {
				n++
			}
		case "emerald", "emeraldTeam":
			if _, ok := st.Item().(item.Emerald); ok {
				n++
			}
		}
	}
	return n
}

// playersNear returns the non-spectator players within r blocks.
func playersNear(tx *world.Tx, pos util.Vec, r float64) []*player.Player {
	var out []*player.Player
	for e := range tx.EntitiesWithin(box(pos, r)) {
		if p, ok := e.(*player.Player); ok && p.GameMode() != world.GameModeSpectator {
			out = append(out, p)
		}
	}
	return out
}

// upgradeGeneratorTier sets the tier of every shared generator of kind.
func (g *Game) upgradeGeneratorTier(kind string, level int) {
	for _, ge := range g.Gens {
		if ge.Kind == kind {
			ge.Level = level
		}
	}
}

// closeTeamGenerators removes the generators within 20 blocks of the team bed.
func (g *Game) closeTeamGenerators(team string) {
	s, ok := g.Data.TeamBed[team]
	if !ok {
		return
	}
	bed := util.MustVec(s)
	keep := g.Gens[:0]
	for _, ge := range g.Gens {
		if ge.Pos.Distance(bed) < 20 {
			closeHandle(ge.label)
			continue
		}
		keep = append(keep, ge)
	}
	g.Gens = keep
}

// closeHandle removes an entity asynchronously.
func closeHandle(h *world.EntityHandle) {
	if h == nil {
		return
	}
	go h.ExecWorld(func(tx *world.Tx, e world.Entity) { _ = e.Close() })
}

// destroyEntity removes the generators and every non-player entity, and clears the per-round
// records.
func (g *Game) destroyEntity() {
	for _, ge := range g.Gens {
		closeHandle(ge.label)
	}
	g.Gens = nil
	if g.World != nil {
		g.World.Exec(func(tx *world.Tx) {
			var victims []world.Entity
			for e := range tx.Entities() {
				if _, isPlayer := e.(*player.Player); !isPlayer {
					victims = append(victims, e)
				}
			}
			for _, e := range victims {
				_ = e.Close()
			}
		})
	}
	g.closeAllMobs()
	g.TempTeam = map[string]string{}
	g.PlacedBlock = map[cube.Pos]bool{}
}

// entityItem creates a dropped item at the player position.
func entityItem(p *player.Player, st item.Stack) *world.EntityHandle {
	return entity.NewItem(world.EntitySpawnOpts{Position: p.Position()}, st)
}

// itoa formats an int.
func itoa(n int) string     { return strconv.Itoa(n) }
// ftoa formats a float without trailing zeros.
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
