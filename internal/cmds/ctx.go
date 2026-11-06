package cmds

import (
	"strings"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/game"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// buildCtx runs inside the command transaction and reads the caller position, the blocks around
// them and their inventory.
func buildCtx(p *player.Player, tx *world.Tx, cmdName string) *Ctx {
	pos := p.Position()
	c := &Ctx{W: tx.World(), Pos: util.Vec{X: pos[0], Y: pos[1], Z: pos[2]}}

	game.P.Mu.Lock()
	c.Pl = game.P.Online[p.Name()]
	game.P.Mu.Unlock()
	if c.Pl == nil {
		c.Pl = plat.Wrap(p)
	}

	isBed := func(v mgl64.Vec3) bool {
		_, ok := tx.Block(cube.PosFromVec3(v)).(block.Bed)
		return ok
	}
	c.OnBed = isBed(pos.Add(mgl64.Vec3{0, -0.5, 0})) || isBed(pos.Add(mgl64.Vec3{0, 0.5, 0})) || isBed(pos)

	switch tx.Block(cube.PosFromVec3(pos.Add(mgl64.Vec3{0, -1, 0}))).(type) {
	case block.Diamond:
		c.BlockBelow = "diamond"
	case block.Emerald:
		c.BlockBelow = "emerald"
	}

	for _, st := range p.Inventory().Slots() {
		if st.Empty() {
			continue
		}
		if w, ok := st.Item().(block.Wool); ok {
			c.Wool = append(c.Wool, woolName(w.Colour.String()))
		}
	}

	// Cache the first bed in the scan so setSpawn can assign it without another transaction lookup.
	if cmdName == "setspawn" {
		const r = 17
		base := cube.PosFromVec3(pos)
	search:
		for x := -r; x < r; x++ {
			for y := -r; y < r; y++ {
				for z := -r; z < r; z++ {
					if _, ok := tx.Block(cube.Pos{base[0] + x, base[1] + y, base[2] + z}).(block.Bed); ok {
						v := util.Vec{X: float64(base[0] + x), Y: float64(base[1] + y), Z: float64(base[2] + z)}
						c.BedFound = &v
						break search
					}
				}
			}
		}
	}
	return c
}

// woolName converts a dye identifier to its display name ("light_blue" becomes "Light Blue").
func woolName(s string) string {
	parts := strings.Split(strings.ReplaceAll(s, "-", "_"), "_")
	for i, p := range parts {
		parts[i] = util.Ucfirst(p)
	}
	return strings.Join(parts, " ")
}
