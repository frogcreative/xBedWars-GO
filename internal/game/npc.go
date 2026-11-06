package game

import (
	"strings"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/util"
)

// NPCType is the entity type of the shop and upgrade villagers. Dragonfly has no villager entity,
// so a stationary custom type is announced to clients as "minecraft:villager_v2".
type NPCType struct{ Kind string }

// Open implements world.EntityType.
func (t NPCType) Open(tx *world.Tx, h *world.EntityHandle, d *world.EntityData) world.Entity {
	return entity.Open(tx, h, d)
}
func (NPCType) EncodeEntity() string                        { return "minecraft:villager_v2" }
func (NPCType) BBox(world.Entity) cube.BBox                 { return cube.Box(-0.3, 0, -0.3, 0.3, 1.95, 0.3) }
func (NPCType) DecodeNBT(map[string]any, *world.EntityData) {}
func (NPCType) EncodeNBT(*world.EntityData) map[string]any  { return nil }

// npcConf configures the NPC entities as stationary.
var npcConf = entity.Config{Behaviour: entity.StationaryBehaviourConfig{}.New()}

// spawnNPC spawns a shop or upgrade villager.
func (g *Game) spawnNPC(kind string, pos util.Vec, tag string) {
	h := npcConf.New(NPCType{Kind: kind}, world.EntitySpawnOpts{
		Position: mgl64.Vec3{pos.X, pos.Y, pos.Z},
		NameTag:  tag,
	})
	g.World.Exec(func(tx *world.Tx) { tx.AddEntity(h) })
}

// initShop spawns the shop and upgrade villagers of every team.
func (g *Game) initShop() {
	for _, s := range g.Data.TeamShop {
		g.spawnNPC("shop", util.MustVec(s).Add(0.5, 0, 0.5), "§bITEM SHOP\n§l§eRIGHT CLICK")
	}
	for _, s := range g.Data.TeamUpgrade {
		tag := "§bTEAM\n§bUPGRADE\n§l§eRIGHT CLICK"
		if m := g.Data.Max(); m == 1 || m == 2 {
			tag = "§b" + strings.ToUpper(util.MaxInTeamToGroup(m)) + "\n§bUPGRADES\n§l§eRIGHT CLICK"
		}
		g.spawnNPC("upgrade", util.MustVec(s).Add(0.5, 0, 0.5), tag)
	}
}
