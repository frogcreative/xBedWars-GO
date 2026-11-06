package game

import (
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/cfg"
	"github.com/frogcreative/bedwars/internal/util"
)

// flight is the result of a simulated projectile throw.
type flight struct {
	Trail []cube.Pos
	Last  mgl64.Vec3
	Ticks int
}

// simulateFlight steps a projectile through the air (gravity 0.03, 1% drag) until it hits a block
// or exceeds maxAge ticks. Every step through air records a plus-shaped group of five positions.
func simulateFlight(tx *world.Tx, start, motion mgl64.Vec3, maxAge int) flight {
	f := flight{Last: start}
	pos := start
	for f.Ticks = 1; f.Ticks <= maxAge; f.Ticks++ {
		next := pos.Add(motion)
		if !isAir(tx, cube.PosFromVec3(next)) {
			break
		}
		pos, f.Last = next, next
		c := cube.PosFromVec3(pos)
		f.Trail = append(f.Trail, c,
			cube.Pos{c[0] - 1, c[1], c[2]}, cube.Pos{c[0], c[1], c[2] - 1},
			cube.Pos{c[0] + 1, c[1], c[2]}, cube.Pos{c[0], c[1], c[2] + 1})
		motion[1] -= 0.03
		motion = motion.Mul(0.99)
	}
	return f
}

// ThrowEgg handles a bridge egg thrown by the player. The caller consumes the item.
func (g *Game) ThrowEgg(p *player.Player) {
	tx := p.Tx()
	eye := p.Position().Add(mgl64.Vec3{0, 1.62, 0})
	motion := DirVec(p.Rotation()).Mul(cfg.Float("game.egg-bridge.throw-power", 1.5))
	maxAge := cfg.Int("game.egg-bridge.max-age-ticks", 60)

	tx.AddEntity(entity.NewEgg(world.EntitySpawnOpts{Position: eye, Velocity: motion}, p))
	f := simulateFlight(tx, eye, motion, maxAge)

	g.P.Mu.Lock()
	pl := g.P.Online[p.Name()]
	team := ""
	if pl != nil {
		team = g.getTeam(pl)
	}
	g.P.Mu.Unlock()
	if team == "" {
		return
	}
	w, col, name := tx.World(), colourFor(team), p.Name()
	time.AfterFunc(time.Duration(f.Ticks)*50*time.Millisecond, func() { g.layBridge(w, f.Trail, col, name) })
}

// ThrowBedbug handles a bedbug snowball. The bedbug spawns where the snowball lands.
func (g *Game) ThrowBedbug(p *player.Player) {
	tx := p.Tx()
	eye := p.Position().Add(mgl64.Vec3{0, 1.62, 0})
	motion := DirVec(p.Rotation()).Mul(1.5)

	tx.AddEntity(entity.NewSnowball(world.EntitySpawnOpts{Position: eye, Velocity: motion}, p))
	f := simulateFlight(tx, eye, motion, 200)
	name := p.Name()
	time.AfterFunc(time.Duration(f.Ticks)*50*time.Millisecond, func() { g.SpawnBedbug(f.Last, name) })
}

// layBridge places the bridge as team wool, skipping the 8-block safe zone around every team spawn
// and any occupied position.
func (g *Game) layBridge(w *world.World, trail []cube.Pos, col item.Colour, owner string) {
	g.P.Mu.Lock()
	if g.Phase != PhaseGame || g.World != w {
		g.P.Mu.Unlock()
		return
	}
	seen := map[cube.Pos]bool{}
	var place []cube.Pos
next:
	for _, p := range trail {
		if seen[p] {
			continue
		}
		seen[p] = true
		bp := util.Vec{X: float64(p[0]), Y: float64(p[1]), Z: float64(p[2])}
		for _, s := range g.Data.TeamSpawn {
			if bp.Distance(util.MustVec(s)) < 8 {
				continue next
			}
		}
		place = append(place, p)
	}
	g.P.Mu.Unlock()

	w.Exec(func(tx *world.Tx) {
		var placed []cube.Pos
		for _, p := range place {
			if _, air := tx.Block(p).(block.Air); !air {
				continue
			}
			tx.SetBlock(p, block.Wool{Colour: col}, nil)
			placed = append(placed, p)
		}
		g.P.Mu.Lock()
		for _, p := range placed {
			g.PlacedBlock[p] = true
		}
		g.P.Mu.Unlock()
	})
}
