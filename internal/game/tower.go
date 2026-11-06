package game

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"

	"github.com/frogcreative/bedwars/internal/util"
)

// towerSteps selects the tower layout facing the player look direction.
func towerSteps(yawDeg float64) []towerStep {
	yaw := yawDeg * math.Pi / 180
	dx, dz := -math.Sin(yaw), math.Cos(yaw)
	if math.Abs(dx) > math.Abs(dz) {
		if dx > 0 {
			return towerEast
		}
		return towerWest
	}
	if dz > 0 {
		return towerSouth
	}
	return towerNorth
}

// ladderDir maps a layout facing code to a ladder direction.
func ladderDir(f int) cube.Direction {
	switch f {
	case 1:
		return cube.North
	case 2:
		return cube.South
	case 3:
		return cube.East
	}
	return cube.West
}

// SpawnTower builds the pop-up tower around the chest, one block per tick. It returns false, after
// telling the player, when the tower would collide with the map. Call it from inside the player
// transaction.
func (g *Game) SpawnTower(p *player.Player, chest cube.Pos) bool {
	tx := p.Tx()
	steps := towerSteps(p.Rotation().Yaw())

	g.P.Mu.Lock()
	pl := g.P.Online[p.Name()]
	if pl == nil || g.Phase != PhaseGame {
		g.P.Mu.Unlock()
		return false
	}
	team := g.getTeam(pl)
	for _, s := range steps {
		pos := cube.Pos{chest[0] + s.dx, chest[1] + s.dy, chest[2] + s.dz}
		if _, air := tx.Block(pos).(block.Air); !air && !g.PlacedBlock[pos] {
			g.P.Mu.Unlock()
			p.Message(util.Msg("messages.popup-tower-blocked"))
			return false
		}
	}
	col := colourFor(team)
	w := tx.World()
	g.P.Mu.Unlock()

	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for _, s := range steps {
			<-t.C
			pos := cube.Pos{chest[0] + s.dx, chest[1] + s.dy, chest[2] + s.dz}
			g.P.Mu.Lock()
			if g.Phase != PhaseGame || g.World != w {
				g.P.Mu.Unlock()
				return
			}
			g.PlacedBlock[pos] = true
			g.P.Mu.Unlock()
			var b world.Block = block.Wool{Colour: col}
			if s.facing != 0 {
				b = block.Ladder{Facing: ladderDir(s.facing)}
			}
			w.Exec(func(tx *world.Tx) { tx.SetBlock(pos, b, nil) })
		}
	}()
	return true
}
