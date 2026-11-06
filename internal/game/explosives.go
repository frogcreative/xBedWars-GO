package game

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/particle"
	"github.com/df-mc/dragonfly/server/world/sound"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/cfg"
	"github.com/frogcreative/bedwars/internal/util"
)

// FXType is a purely visual entity (primed TNT, fireball, mobs). Behavior is driven by game code.
type FXType struct {
	ID   string
	W, H float64
}

// Open implements world.EntityType.
func (t FXType) Open(tx *world.Tx, h *world.EntityHandle, d *world.EntityData) world.Entity {
	return entity.Open(tx, h, d)
}
// EncodeEntity returns the Bedrock entity identifier.
func (t FXType) EncodeEntity() string { return t.ID }
// BBox returns the entity hit box.
func (t FXType) BBox(world.Entity) cube.BBox {
	return cube.Box(-t.W/2, 0, -t.W/2, t.W/2, t.H, t.W/2)
}
func (FXType) DecodeNBT(map[string]any, *world.EntityData) {}
func (FXType) EncodeNBT(*world.EntityData) map[string]any  { return nil }

// Entity types and configurations of the explosives.
var (
	tntType  = FXType{ID: "minecraft:tnt", W: 0.98, H: 0.98}
	fireType = FXType{ID: "minecraft:fireball", W: 0.5, H: 0.5}

	tntConf  = entity.Config{Behaviour: entity.StationaryBehaviourConfig{}.New()}
	fireConf = entity.Config{Behaviour: entity.PassiveBehaviourConfig{Gravity: 0, Drag: 0, ExistenceDuration: 10 * time.Second}.New()}
)

// IsFireball reports whether t is the fireball entity type.
func IsFireball(t world.EntityType) bool { _, ok := t.(FXType); return ok && t.(FXType).ID == fireType.ID }

// DirVec returns the unit look direction for a rotation.
func DirVec(rot cube.Rotation) mgl64.Vec3 {
	yaw, pitch := rot.Yaw()*math.Pi/180, rot.Pitch()*math.Pi/180
	return mgl64.Vec3{-math.Sin(yaw) * math.Cos(pitch), -math.Sin(pitch), math.Cos(yaw) * math.Cos(pitch)}
}

// SpawnTNT lights a TNT block at pos with the given fuse in ticks. Call it from inside the player
// transaction.
func (g *Game) SpawnTNT(p *player.Player, pos cube.Pos, fuseTicks int) {
	tx := p.Tx()
	centre := mgl64.Vec3{float64(pos[0]) + 0.5, float64(pos[1]), float64(pos[2]) + 0.5}
	h := tntConf.New(tntType, world.EntitySpawnOpts{Position: centre})
	tx.AddEntity(h)
	owner, w := p.Name(), tx.World()
	time.AfterFunc(time.Duration(fuseTicks)*50*time.Millisecond, func() {
		closeHandle(h)
		g.explodeAt(w, centre.Add(mgl64.Vec3{0, 0.49, 0}), owner, "tnt")
	})
}

// SpawnFireball launches a fireball at 1.5 blocks per tick in the look direction. Call it from
// inside the player transaction.
func (g *Game) SpawnFireball(p *player.Player) {
	tx := p.Tx()
	eye := p.Position().Add(mgl64.Vec3{0, 1.62, 0})
	dir := DirVec(p.Rotation())
	h := fireConf.New(fireType, world.EntitySpawnOpts{Position: eye, Velocity: dir.Mul(1.5)})
	tx.AddEntity(h)
	go g.trackFireball(h, tx.World(), p.Name())
}

// trackFireball polls the fireball every tick and detonates it on contact with a block or another
// player (200 ticks maximum).
func (g *Game) trackFireball(h *world.EntityHandle, w *world.World, owner string) {
	for i := 0; i < 200; i++ {
		time.Sleep(50 * time.Millisecond)
		hit := false
		var centre mgl64.Vec3
		alive := h.ExecWorld(func(tx *world.Tx, e world.Entity) {
			pos := e.Position()
			ahead := pos
			if ent, ok := e.(*entity.Ent); ok {
				ahead = pos.Add(ent.Velocity().Mul(0.5))
			}
			solid := func(v mgl64.Vec3) bool { _, air := tx.Block(cube.PosFromVec3(v)).(block.Air); return !air }
			if solid(pos) || solid(ahead) {
				hit = true
			}
			for o := range tx.EntitiesWithin(box(util.Vec{X: pos[0], Y: pos[1], Z: pos[2]}, 1)) {
				if pl, ok := o.(*player.Player); ok && (pl.Name() != owner || i > 5) {
					hit = true
				}
			}
			if hit {
				centre = pos
				_ = e.Close()
			}
		})
		if !alive {
			return
		}
		if hit {
			g.explodeAt(w, centre, owner, "fireball")
			return
		}
	}
	closeHandle(h)
}

// explodeAt schedules an explosion on the world goroutine.
func (g *Game) explodeAt(w *world.World, centre mgl64.Vec3, owner, kind string) {
	w.Exec(func(tx *world.Tx) { g.explodeTx(tx, centre, owner, kind) })
}

// victim is a player affected by an explosion.
type victim struct {
	p        *player.Player
	name     string
	friendly bool
}

// explodeTx applies an explosion on the world goroutine: it destroys player-placed blocks (never
// glass) within the radius, knocks back players in range and damages those who are not on the
// owner team.
func (g *Game) explodeTx(tx *world.Tx, centre mgl64.Vec3, owner, kind string) {
	base := "game.explosives." + kind + "."
	radius := cfg.Float(base+"radius", 3)
	rng := cfg.Float(base+"damage-range", 6)
	dmg := cfg.Float(base+"direct-damage", 2)
	kb := cfg.Float(base+"knockback", 2)

	// Snapshot the player-placed blocks and team data under the lock, then modify the world without
	// it.
	g.P.Mu.Lock()
	if g.Phase != PhaseGame {
		g.P.Mu.Unlock()
		return
	}
	var targets []cube.Pos
	c := cube.PosFromVec3(centre)
	r := int(math.Ceil(radius))
	for x := -r; x <= r; x++ {
		for y := -r; y <= r; y++ {
			for z := -r; z <= r; z++ {
				if math.Sqrt(float64(x*x+y*y+z*z)) > radius {
					continue
				}
				p := cube.Pos{c[0] + x, c[1] + y, c[2] + z}
				if g.PlacedBlock[p] {
					targets = append(targets, p)
				}
			}
		}
	}
	ownerTeam := ""
	if o := g.P.Online[owner]; o != nil {
		ownerTeam = g.getTeam(o)
	}
	g.P.Mu.Unlock()

	for _, p := range targets {
		switch tx.Block(p).(type) {
		case block.Glass, block.StainedGlass, block.Air:
			continue
		}
		tx.SetBlock(p, block.Air{}, nil)
	}
	tx.PlaySound(centre, sound.Explosion{})
	tx.AddParticle(centre, particle.HugeExplosion{})

	var victims []victim
	for e := range tx.EntitiesWithin(box(util.Vec{X: centre[0], Y: centre[1], Z: centre[2]}, rng+1)) {
		pl, ok := e.(*player.Player)
		if !ok || pl.Position().Sub(centre).Len() > rng {
			continue
		}
		victims = append(victims, victim{p: pl, name: pl.Name()})
	}
	g.P.Mu.Lock()
	for i := range victims {
		v := &victims[i]
		on := g.P.Online[v.name]
		_, spec := g.Spectators[v.name]
		if on == nil || spec {
			v.friendly = true
			continue
		}
		if t := g.getTeam(on); t == ownerTeam {
			v.friendly = true
			continue
		}
		g.LastDamager[v.name] = owner
		g.CauseOverride[v.name] = "projectile"
	}
	g.P.Mu.Unlock()

	for _, v := range victims {
		dir := v.p.Position().Sub(centre)
		if dir.Len() > 0 {
			dir = dir.Normalize()
		}
		if !v.friendly {
			// Teammates, including the owner, are knocked back but not damaged.
			v.p.Hurt(dmg, block.ExplosionDamageSource{})
		}
		v.p.SetVelocity(dir.Mul(kb))
	}
}
