package game

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/util"
)

// mobKind describes one of the team-owned combat mobs.
type mobKind struct {
	Label     string
	Type      FXType
	HP        float64
	Damage    float64
	MaxTarget float64
}

// Mob definitions and entity configuration.
var (
	bedbugKind = mobKind{Label: "Bed Bug", Type: FXType{ID: "minecraft:silverfish", W: 0.5, H: 0.5}, HP: 20, Damage: 1, MaxTarget: 100}
	golemKind  = mobKind{Label: "Dream Defender", Type: FXType{ID: "minecraft:iron_golem", W: 1.4, H: 2.7}, HP: 60, Damage: 3, MaxTarget: 30}

	mobConf = entity.Config{Behaviour: entity.PassiveBehaviourConfig{Gravity: 0.08, Drag: 0.02, ExistenceDuration: time.Hour}.New()}
)

// Mob behavior constants. The original entities applied their motion twice per tick, so the
// effective walking speed is twice the nominal 0.2 * 0.15.
const (
	mobLifetimeSeconds = 120
	mobAttackDelay     = 16
	mobSpeed = 0.2 * 0.15 * 2
)

// Mob is the server-side state of a bedbug or dream defender.
type Mob struct {
	Kind     mobKind
	H        *world.EntityHandle
	Owner    string
	Team     string
	HP       float64
	Life     int
	AttackCD int
	Pos      util.Vec
}

// MobAttackSource is the damage source used by mobs. Its type name contains "Attack", which the
// event handler classifies as a melee kill credited to the mob owner.
type MobAttackSource struct{}

func (MobAttackSource) ReducedByArmour() bool     { return true }
func (MobAttackSource) ReducedByResistance() bool { return true }
func (MobAttackSource) Fire() bool                { return false }
func (MobAttackSource) IgnoreTotem() bool         { return false }

// IsMob reports whether t is the entity type of a combat mob.
func IsMob(t world.EntityType) bool {
	fx, ok := t.(FXType)
	return ok && (fx.ID == bedbugKind.Type.ID || fx.ID == golemKind.Type.ID)
}

// SpawnBedbug spawns a bedbug owned by the given player.
func (g *Game) SpawnBedbug(pos mgl64.Vec3, owner string) { g.spawnMob(bedbugKind, pos, owner) }

// SpawnDreamDefender spawns an iron golem owned by the given player.
func (g *Game) SpawnDreamDefender(pos mgl64.Vec3, owner string) { g.spawnMob(golemKind, pos, owner) }

// spawnMob registers and spawns a mob, then starts its AI loop.
func (g *Game) spawnMob(kind mobKind, pos mgl64.Vec3, owner string) {
	g.P.Mu.Lock()
	pl := g.P.Online[owner]
	if g.Phase != PhaseGame || pl == nil || g.World == nil {
		g.P.Mu.Unlock()
		return
	}
	h := mobConf.New(kind.Type, world.EntitySpawnOpts{Position: pos})
	m := &Mob{Kind: kind, H: h, Owner: owner, Team: g.getTeam(pl), HP: kind.HP, Life: mobLifetimeSeconds, Pos: util.Vec{X: pos[0], Y: pos[1], Z: pos[2]}}
	g.Mobs[h] = m
	w := g.World
	g.P.Mu.Unlock()

	w.Exec(func(tx *world.Tx) { tx.AddEntity(h) })
	go g.runMob(m)
}

// DamageMob applies melee damage from a player to a mob. It returns false when the attack is
// ignored (friendly fire or unknown mob).
func (g *Game) DamageMob(h *world.EntityHandle, attacker string, dmg float64) bool {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	m, ok := g.Mobs[h]
	if !ok {
		return false
	}
	if a := g.P.Online[attacker]; a == nil || g.getTeam(a) == m.Team {
		return false
	}
	m.HP -= dmg
	if m.HP <= 0 {
		delete(g.Mobs, h)
		closeHandle(h)
	}
	return true
}

// mobTarget is a candidate victim of a mob.
type mobTarget struct {
	Name   string
	Pos    util.Vec
	Player bool
	Mob    *Mob
}

// targets collects every valid enemy of the mob. The caller must hold the plugin lock.
func (g *Game) targets(m *Mob) []mobTarget {
	var out []mobTarget
	for _, pt := range g.Players {
		if !pt.Online() || g.getTeam(pt) == m.Team {
			continue
		}
		if _, sp := g.Spectators[pt.Name]; sp {
			continue
		}
		if _, rs := g.RespawnC[pt.Name]; rs {
			continue
		}
		if pos, ok := g.P.Pos[pt.Name]; ok {
			out = append(out, mobTarget{Name: pt.Name, Pos: pos, Player: true})
		}
	}
	// Enemy mobs are targets too, so defenders can fight opposing defenders.
	for _, o := range g.Mobs {
		if o != m && o.Team != m.Team {
			out = append(out, mobTarget{Name: o.Kind.Label, Pos: o.Pos, Mob: o})
		}
	}
	return out
}

// runMob drives a mob at 20 ticks per second until it dies, expires or the match ends. It targets
// the nearest enemy, walks towards it, jumps over one-block obstacles and attacks every 16 ticks
// on contact.
func (g *Game) runMob(m *Mob) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	secondTick := 0
	for range ticker.C {
		g.P.Mu.Lock()
		owner := g.P.Online[m.Owner]
		if _, alive := g.Mobs[m.H]; !alive || g.Phase != PhaseGame || owner == nil || !owner.Online() {
			delete(g.Mobs, m.H)
			g.P.Mu.Unlock()
			closeHandle(m.H)
			return
		}
		secondTick++
		tag := ""
		if secondTick >= 20 {
			secondTick = 0
			m.Life--
			tag = util.GetChatColor(m.Team) + "§l" + m.Kind.Label + " §r§7[" + itoa(m.Life) + "]\n§r§f" + ftoa(m.HP)
		}
		if m.Life <= 0 {
			delete(g.Mobs, m.H)
			g.P.Mu.Unlock()
			closeHandle(m.H)
			return
		}
		// Everything that needs the lock happens before the world call; the world closure below never
		// takes it.
		targets := g.targets(m)
		cooldown := m.AttackCD
		g.P.Mu.Unlock()

		var hit *mobTarget
		var pos mgl64.Vec3
		ok := m.H.ExecWorld(func(tx *world.Tx, e world.Entity) {
			pos = e.Position()
			ent, isEnt := e.(*entity.Ent)
			if !isEnt {
				return
			}
			if tag != "" {
				ent.SetNameTag(tag)
			}
			best, bestDist := -1, math.MaxFloat64
			for i, t := range targets {
				if d := pos.Sub(mgl64.Vec3{t.Pos.X, t.Pos.Y, t.Pos.Z}).Len(); d <= m.Kind.MaxTarget && d < bestDist {
					best, bestDist = i, d
				}
			}
			if best < 0 {
				return
			}
			tg := targets[best]
			dx, dz := tg.Pos.X-pos[0], tg.Pos.Z-pos[2]
			vel := ent.Velocity()
			if dx*dx+dz*dz < 0.7 {
				vel[0], vel[2] = 0, 0
			} else if diff := math.Abs(dx) + math.Abs(dz); diff > 0 {
				vel[0], vel[2] = mobSpeed*dx/diff, mobSpeed*dz/diff
			}
			if math.Abs(vel[1]) < 0.01 {
				ahead := cube.PosFromVec3(pos.Add(mgl64.Vec3{vel[0] * 10, 0.5, vel[2] * 10}))
				if !isAir(tx, ahead) && isAir(tx, ahead.Side(cube.FaceUp)) && isAir(tx, ahead.Side(cube.FaceUp).Side(cube.FaceUp)) {
					vel[1] = 0.42
				}
			}
			ent.SetVelocity(vel)
			if cooldown > mobAttackDelay && dx*dx+dz*dz < 1.5 && math.Abs(tg.Pos.Y-pos[1]) < 1.8 {
				hit = &tg
			}
		})
		if !ok {
			g.P.Mu.Lock()
			delete(g.Mobs, m.H)
			g.P.Mu.Unlock()
			return
		}

		g.P.Mu.Lock()
		m.Pos = util.Vec{X: pos[0], Y: pos[1], Z: pos[2]}
		if hit != nil {
			m.AttackCD = 0
			g.applyMobHit(m, *hit)
		} else {
			m.AttackCD++
		}
		g.P.Mu.Unlock()
	}
}

// applyMobHit resolves a mob attack. The caller must hold the plugin lock.
func (g *Game) applyMobHit(m *Mob, t mobTarget) {
	if t.Mob != nil {
		t.Mob.HP -= m.Kind.Damage
		if t.Mob.HP <= 0 {
			delete(g.Mobs, t.Mob.H)
			closeHandle(t.Mob.H)
		}
		return
	}
	victim := g.P.Online[t.Name]
	if victim == nil {
		return
	}
	g.LastDamager[t.Name] = m.Owner
	dmg := m.Kind.Damage
	victim.Do(func(_ *world.Tx, p *player.Player) { p.Hurt(dmg, MobAttackSource{}) })
}

// isAir reports whether the block at p is air.
func isAir(tx *world.Tx, p cube.Pos) bool {
	_, ok := tx.Block(p).(block.Air)
	return ok
}

// closeAllMobs forgets every tracked mob; the entities themselves are removed by destroyEntity.
func (g *Game) closeAllMobs() { g.Mobs = map[*world.EntityHandle]*Mob{} }

