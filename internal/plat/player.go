// Package plat is the only place where game logic touches the Dragonfly player API.
//
// Dragonfly runs each player inside its world transaction. Every action here is queued and
// executed asynchronously, so game code holding the plugin lock never waits on a world goroutine.
// Data that must be read from a player (position, health) has to be captured by the event or
// command that is already running inside the transaction.
package plat

import (
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/enchantment"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/player/scoreboard"
	"github.com/df-mc/dragonfly/server/player/title"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

// Player wraps a player handle together with an ordered action queue.
type Player struct {
	Name string
	H    *world.EntityHandle

	// mu keeps enqueueing and channel closure mutually exclusive.
	mu     sync.Mutex
	q      chan func(tx *world.Tx, p *player.Player)
	online atomic.Bool
}

// Wrap creates the wrapper and starts its queue worker. Call it once per player when they join.
func Wrap(p *player.Player) *Player {
	w := &Player{Name: p.Name(), H: p.H(), q: make(chan func(*world.Tx, *player.Player), 512)}
	w.online.Store(true)
	go w.loop()
	return w
}

// loop executes queued actions in order inside the player transaction.
func (p *Player) loop() {
	for f := range p.q {
		if !p.online.Load() {
			continue
		}
		// A player is briefly in no world while changing worlds; retry for up to two seconds.
		for i := 0; i < 100; i++ {
			if p.H.ExecWorld(func(tx *world.Tx, e world.Entity) { f(tx, e.(*player.Player)) }) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Close stops the queue. Pending actions are discarded. Call it when the player quits.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.online.Swap(false) {
		close(p.q)
	}
}

// Online reports whether the player is still connected.
func (p *Player) Online() bool { return p.online.Load() }

// Do queues f to run inside the player transaction. It never blocks; the action is dropped when
// the queue is full.
func (p *Player) Do(f func(tx *world.Tx, pl *player.Player)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.online.Load() {
		return
	}
	select {
	// The queue is bounded so a stalled player can never block game logic.
	case p.q <- f:
	default:
	}
}

// Msg sends a chat message.
func (p *Player) Msg(s string) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.Message(s) })
}

// Title shows a title with the given fade and stay durations.
func (p *Player) Title(t, sub string, fadeIn, stay, fadeOut time.Duration) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		tt := title.New(t).WithFadeInDuration(fadeIn).WithDuration(stay).WithFadeOutDuration(fadeOut)
		if sub != "" {
			tt = tt.WithSubtitle(sub)
		}
		pl.SendTitle(tt)
	})
}

// Title20 shows a title with the default timings (0.5 s fade in, 1 s stay, 0.25 s fade out).
func (p *Player) Title20(t, sub string) {
	p.Title(t, sub, 500*time.Millisecond, time.Second, 250*time.Millisecond)
}

// Subtitle shows only a subtitle.
func (p *Player) Subtitle(s string) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.SendTitle(title.New(" ").WithSubtitle(s)) })
}

// Sound plays a named sound. Dragonfly only exposes a fixed sound set, so this is currently a no-
// op.
func (p *Player) Sound(name string) { _ = name }

// SetScoreboard replaces the sidebar scoreboard.
func (p *Player) SetScoreboard(titleText string, lines []string) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		sb := scoreboard.New(titleText)
		for _, l := range lines {
			_, _ = sb.WriteString(l + "\n")
		}
		pl.SendScoreboard(sb)
	})
}

// RemoveScoreboard hides the sidebar scoreboard.
func (p *Player) RemoveScoreboard() {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.RemoveScoreboard() })
}

// SetScoreTag sets the text shown below the name tag.
func (p *Player) SetScoreTag(s string) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.SetScoreTag(s) })
}

// HealthTag shows the current health and symbol below the name tag.
func (p *Player) HealthTag(symbol string) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		hp := strconv.FormatFloat(pl.Health(), 'f', -1, 64)
		pl.SetScoreTag("§f" + hp + " §c" + symbol)
	})
}

// SetNameTag sets the name tag.
func (p *Player) SetNameTag(s string) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.SetNameTag(s) })
}

// SetGameMode accepts "survival", "creative", "adventure" or "spectator".
func (p *Player) SetGameMode(mode string) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		switch strings.ToLower(mode) {
		case "survival":
			pl.SetGameMode(world.GameModeSurvival)
		case "creative":
			pl.SetGameMode(world.GameModeCreative)
		case "adventure":
			pl.SetGameMode(world.GameModeAdventure)
		case "spectator":
			pl.SetGameMode(world.GameModeSpectator)
		}
	})
}

// Restore refills health and food.
func (p *Player) Restore() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		pl.SetFood(20)
		if pl.Health() < pl.MaxHealth() {
			pl.Heal(pl.MaxHealth()-pl.Health(), entity.FoodHealingSource{})
		}
	})
}

// SetFood sets the food level.
func (p *Player) SetFood(n int) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.SetFood(n) })
}

// ClearEffects removes every active effect.
func (p *Player) ClearEffects() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		for _, e := range pl.Effects() {
			pl.RemoveEffect(e.Type())
		}
	})
}

// NewEffect builds an effect from its name: speed, haste, regeneration, invisibility, jump,
// blindness, slowness or miningfatigue.
func NewEffect(kind string, level int, dur time.Duration, visible bool) (effect.Effect, bool) {
	var t effect.Type
	switch kind {
	case "speed":
		t = effect.Speed
	case "haste":
		t = effect.Haste
	case "regeneration":
		t = effect.Regeneration
	case "invisibility":
		t = effect.Invisibility
	case "jump":
		t = effect.JumpBoost
	case "blindness":
		t = effect.Blindness
	case "slowness":
		t = effect.Slowness
	case "miningfatigue":
		t = effect.MiningFatigue
	default:
		return effect.Effect{}, false
	}
	e := effect.New(t, level, dur)
	if !visible {
		e = e.WithoutParticles()
	}
	return e, true
}

// AddEffect applies an effect.
func (p *Player) AddEffect(kind string, level int, dur time.Duration, visible bool) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		if e, ok := NewEffect(kind, level, dur, visible); ok {
			pl.AddEffect(e)
		}
	})
}

// ClearInventories empties the inventory, armour and ender chest and selects the first hotbar
// slot.
func (p *Player) ClearInventories() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		pl.Inventory().Clear()
		pl.Armour().Clear()
		pl.EnderChestInventory().Clear()
		pl.SetHeldSlot(0)
	})
}

// ClearInventory empties the main inventory only.
func (p *Player) ClearInventory() {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.Inventory().Clear() })
}

// LobbyItems gives the team selector (slot 0) and the leave bed (slot 8).
func (p *Player) LobbyItems() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		_ = pl.Inventory().SetItem(8, item.NewStack(block.Bed{Colour: item.ColourRed()}, 1).WithCustomName("§aReturn to lobby"))
		_ = pl.Inventory().SetItem(0, item.NewStack(block.NewChest(), 1).WithCustomName("Select Team"))
	})
}

// StopFlying disables flight.
func (p *Player) StopFlying() {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.StopFlying() })
}

// TeleportTo moves the player to pos in w, transferring them between worlds when needed.
func (p *Player) TeleportTo(w *world.World, pos mgl64.Vec3) {
	p.Do(func(tx *world.Tx, pl *player.Player) {
		if tx.World() == w {
			pl.Teleport(pos)
			return
		}
		h := tx.RemoveEntity(pl)
		go func() {
			<-w.Exec(func(tx2 *world.Tx) {
				e := tx2.AddEntity(h)
				if np, ok := e.(*player.Player); ok {
					np.Teleport(pos)
				}
			})
		}()
	})
}

// Sync blocks until every previously queued action has run (3 s timeout). Never call it while
// holding the plugin lock.
func (p *Player) Sync() {
	done := make(chan struct{})
	// The no-op runs after earlier queued actions, unless Do drops it on a full queue.
	p.Do(func(_ *world.Tx, _ *player.Player) { close(done) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// SetAbsorption sets the absorption health.
func (p *Player) SetAbsorption(v float64) {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.SetAbsorption(v) })
}

// SetArmour equips team-colored leather armor; the leggings and boots use the given tier
// ("leather", "chainmail", "iron" or "diamond"). prot is the Protection enchantment level, 0 for
// none.
func (p *Player) SetArmour(tier string, r, g, b uint8, prot int) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		col := color.RGBA{R: r, G: g, B: b, A: 255}
		leather := item.ArmourTierLeather{Colour: col}
		var lower item.ArmourTier = leather
		switch tier {
		case "chainmail":
			lower = item.ArmourTierChain{}
		case "iron":
			lower = item.ArmourTierIron{}
		case "diamond":
			lower = item.ArmourTierDiamond{}
		}
		ench := func(s item.Stack) item.Stack {
			if prot > 0 {
				return s.WithEnchantments(item.NewEnchantment(enchantment.Protection, prot))
			}
			return s
		}
		a := pl.Armour()
		a.Clear()
		a.SetHelmet(ench(item.NewStack(item.Helmet{Tier: leather}, 1)))
		a.SetChestplate(ench(item.NewStack(item.Chestplate{Tier: leather}, 1)))
		a.SetLeggings(ench(item.NewStack(item.Leggings{Tier: lower}, 1)))
		a.SetBoots(ench(item.NewStack(item.Boots{Tier: lower}, 1)))
	})
}

// GiveSword places a wooden sword in slot 0, with Sharpness I when sharp is set.
func (p *Player) GiveSword(sharp bool) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		s := item.NewStack(item.Sword{Tier: item.ToolTierWood}, 1)
		if sharp {
			s = s.WithEnchantments(item.NewEnchantment(enchantment.Sharpness, 1))
		}
		_ = pl.Inventory().SetItem(0, s)
	})
}

// ReturnBed gives the "Return to lobby" bed in slot 8.
func (p *Player) ReturnBed() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		_ = pl.Inventory().SetItem(8, item.NewStack(block.Bed{Colour: item.ColourRed()}, 1).WithCustomName("§aReturn to lobby"))
	})
}

// StartFlying enables flight.
func (p *Player) StartFlying() {
	p.Do(func(_ *world.Tx, pl *player.Player) { pl.StartFlying() })
}

// SetHeldSlot selects a hotbar slot.
func (p *Player) SetHeldSlot(n int) {
	p.Do(func(_ *world.Tx, pl *player.Player) { _ = pl.SetHeldSlot(n) })
}

// ReplaceTool removes every axe (or pickaxe) from the inventory and adds st.
func (p *Player) ReplaceTool(isAxe bool, st item.Stack) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		inv := pl.Inventory()
		for slot, s := range inv.Slots() {
			if s.Empty() {
				continue
			}
			switch s.Item().(type) {
			case item.Axe:
				if isAxe {
					_ = inv.SetItem(slot, item.Stack{})
				}
			case item.Pickaxe:
				if !isAxe {
					_ = inv.SetItem(slot, item.Stack{})
				}
			}
		}
		_, _ = inv.AddItem(st)
	})
}

// RemoveEffect removes one effect by name (currently "invisibility").
func (p *Player) RemoveEffect(kind string) {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		if kind == "invisibility" {
			pl.RemoveEffect(effect.Invisibility)
		}
	})
}

// SharpenSwords enchants every sword in the inventory with Sharpness I.
func (p *Player) SharpenSwords() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		inv := pl.Inventory()
		for slot, st := range inv.Slots() {
			if _, ok := st.Item().(item.Sword); ok {
				_ = inv.SetItem(slot, st.WithEnchantments(item.NewEnchantment(enchantment.Sharpness, 1)))
			}
		}
	})
}

// SpectatorCompass gives the spectator navigation compass in slot 4.
func (p *Player) SpectatorCompass() {
	p.Do(func(_ *world.Tx, pl *player.Player) {
		_ = pl.Inventory().SetItem(4, item.NewStack(item.Compass{}, 1).WithCustomName("§eSpectator"))
	})
}
