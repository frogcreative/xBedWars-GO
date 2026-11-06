// Package events translates Dragonfly player events into game rules: lobby behavior, block rules,
// combat, shops, items and chat.
package events

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/player/form"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/game"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Handler is attached to one player and forwards their events to the game.
type Handler struct {
	player.NopHandler
	pl *plat.Player
}

// Attach registers a joined player and installs their handler.
func Attach(p *player.Player) {
	pl := plat.Wrap(p)
	game.P.PlayerOnline(pl)
	p.Handle(&Handler{pl: pl})
}

// HandleQuit removes the player from the game state.
func (h *Handler) HandleQuit(p *player.Player) {
	name := h.pl.Name
	go game.P.PlayerOffline(name)
}

// HandleMove records the position and runs trap and void checks.
func (h *Handler) HandleMove(_ *player.Context, pos mgl64.Vec3, _ cube.Rotation) {
	v := util.Vec{X: pos[0], Y: pos[1], Z: pos[2]}
	game.P.Mu.Lock()
	game.P.Pos[h.pl.Name] = v
	if g := game.P.InGame[h.pl.Name]; g != nil {
		g.OnMove(h.pl, v)
	}
	game.P.Mu.Unlock()
}

// gameOf returns the arena the player is in, or nil.
func (h *Handler) gameOf() *game.Game {
	game.P.Mu.Lock()
	defer game.P.Mu.Unlock()
	return game.P.InGame[h.pl.Name]
}

// HandleBlockBreak enforces the block-breaking rules and handles bed destruction.
func (h *Handler) HandleBlockBreak(ctx *player.Context, pos cube.Pos, drops *[]item.Stack, xp *int) {
	g := h.gameOf()
	if g == nil {
		return
	}
	p := ctx.Val()
	bed, isBed := p.Tx().Block(pos).(block.Bed)
	other := pos
	if isBed {
		other = pos.Side(bed.Facing.Face())
		if bed.Head {
			other = pos.Side(bed.Facing.Opposite().Face())
		}
	}
	cancel, noDrops := g.BreakCheck(h.pl, pos, other, isBed)
	if cancel {
		ctx.Cancel()
		return
	}
	*xp = 0
	if noDrops {
		*drops = nil
	}
}

// HandleBlockPlace enforces the block-placing rules and turns TNT and chests into their special
// behavior.
func (h *Handler) HandleBlockPlace(ctx *player.Context, pos cube.Pos, b world.Block) {
	g := h.gameOf()
	if g == nil {
		return
	}
	special := ""
	switch b.(type) {
	case block.TNT:
		special = "tnt"
	case block.Chest:
		special = "chest"
	}
	cancel, handled := g.PlaceCheck(h.pl, pos, special)
	if cancel {
		ctx.Cancel()
	}
	if !handled {
		return
	}
	p := ctx.Val()
	switch special {
	case "tnt":
		consumeHeld(p)
		g.SpawnTNT(p, pos, 50)
	case "chest":
		if g.SpawnTower(p, pos) {
			consumeHeld(p)
		}
	}
}

// consumeHeld removes one item from the held stack.
func consumeHeld(p *player.Player) {
	main, off := p.HeldItems()
	p.SetHeldItems(main.Grow(-1), off)
}

// isTool reports whether the stack is equipment that cannot be dropped.
func isTool(st item.Stack) bool {
	switch st.Item().(type) {
	case item.Sword, item.Helmet, item.Chestplate, item.Leggings, item.Boots, item.Shears, item.Pickaxe, item.Axe:
		return true
	}
	return false
}

// HandleItemDrop prevents dropping equipment and dropping items outside a running match.
func (h *Handler) HandleItemDrop(ctx *player.Context, st item.Stack) {
	if g := h.gameOf(); g != nil && !g.ItemDropAllowed(h.pl, isTool(st)) {
		ctx.Cancel()
	}
}

// classify maps a Dragonfly damage source to a cause (attack, projectile, block_explosion, fall,
// void, other) and the attacking player name, if any.
func classify(src world.DamageSource) (cause, attacker string) {
	switch s := src.(type) {
	case entity.AttackDamageSource:
		if a, ok := s.Attacker.(*player.Player); ok {
			attacker = a.Name()
		}
	case entity.ProjectileDamageSource:
		if a, ok := s.Owner.(*player.Player); ok {
			attacker = a.Name()
		}
	}
	t := fmt.Sprintf("%T", src)
	switch {
	case strings.Contains(t, "Void"):
		cause = "void"
	case strings.Contains(t, "Fall"):
		cause = "fall"
	case strings.Contains(t, "Projectile"):
		cause = "projectile"
	case strings.Contains(t, "Explosion"):
		cause = "block_explosion"
	case strings.Contains(t, "Attack"):
		cause = "attack"
	default:
		cause = "other"
	}
	return
}

// shouldDrop reports whether the stack is dropped when the player is eliminated: resources,
// potions and building blocks.
func shouldDrop(st item.Stack) bool {
	switch it := st.Item().(type) {
	case item.Diamond, item.GoldIngot, item.IronIngot, item.Emerald, item.Potion, item.GoldenApple,
		item.FireCharge, item.Snowball, item.Egg:
		return true
	case block.Wool, block.Obsidian, block.EndStone, block.Ladder, block.TNT, block.GlazedTerracotta:
		return true
	case block.Planks:
		return it.Wood == block.OakWood()
	}
	return false
}

// dropDeathItems drops the resources and building blocks of an eliminated player.
func dropDeathItems(p *player.Player) {
	tx := p.Tx()
	for _, st := range p.Inventory().Slots() {
		if st.Empty() || !shouldDrop(st) {
			continue
		}
		tx.AddEntity(entity.NewItem(world.EntitySpawnOpts{Position: p.Position()}, st))
	}
}

// HandleHurt applies damage rules and turns lethal damage into a respawn or final kill.
func (h *Handler) HandleHurt(ctx *player.Context, damage *float64, _ bool, _ *time.Duration, src world.DamageSource) {
	g := h.gameOf()
	if g == nil {
		return
	}
	p := ctx.Val()
	cause, attacker := classify(src)
	cancel, final := g.Hurt(h.pl, cause, attacker, p.Health(), *damage)
	if cancel {
		ctx.Cancel()
	}
	if final {
		dropDeathItems(p)
	}
}

// HandleAttackEntity blocks friendly fire and routes hits on mobs and fireballs.
func (h *Handler) HandleAttackEntity(ctx *player.Context, e world.Entity, _, _ *float64, _ *bool) {
	g := h.gameOf()
	if g == nil {
		return
	}
	if game.IsMob(e.H().Type()) {
		dmg := 1.0
		if main, _ := ctx.Val().HeldItems(); main.Item() != nil {
			if w, ok := main.Item().(item.Weapon); ok {
				dmg = w.AttackDamage()
			}
		}
		g.DamageMob(e.H(), h.pl.Name, dmg)
		ctx.Cancel()
		return
	}
	if game.IsFireball(e.H().Type()) {
		if ent, ok := e.(*entity.Ent); ok {
			ent.SetVelocity(game.DirVec(ctx.Val().Rotation()).Mul(0.5))
		}
		ctx.Cancel()
		return
	}
	v, ok := e.(*player.Player)
	if !ok {
		return
	}
	if !g.AttackAllowed(h.pl.Name, v.Name()) {
		ctx.Cancel()
	}
}

// HandleFoodLoss disables hunger inside arenas.
func (h *Handler) HandleFoodLoss(ctx *player.Context, _ int, _ *int) {
	if h.gameOf() != nil {
		ctx.Cancel()
	}
}

// HandleItemUseOnEntity opens the shop and upgrade menus when an NPC is clicked.
func (h *Handler) HandleItemUseOnEntity(ctx *player.Context, e world.Entity) {
	t, ok := e.H().Type().(game.NPCType)
	if !ok {
		return
	}
	g := h.gameOf()
	if g == nil || g.PhaseNow() != game.PhaseGame {
		return
	}
	ctx.Cancel()
	p := ctx.Val()
	switch t.Kind {
	case "shop":
		g.OpenShop(p, "")
	case "upgrade":
		g.OpenUpgrade(p)
	}
}

// useWeapon launches fireballs, bridge eggs and bedbug snowballs. It reports whether the use was
// handled.
func (h *Handler) useWeapon(ctx *player.Context) bool {
	p := ctx.Val()
	main, _ := p.HeldItems()
	g := h.gameOf()
	if g == nil || !g.Playing(h.pl.Name) {
		return false
	}
	switch main.Item().(type) {
	case item.FireCharge:
		ctx.Cancel()
		consumeHeld(p)
		g.SpawnFireball(p)
		return true
	case item.Egg:
		ctx.Cancel()
		consumeHeld(p)
		g.ThrowEgg(p)
		return true
	case item.Snowball:
		if main.CustomName() == "§eBedbug" {
			ctx.Cancel()
			consumeHeld(p)
			g.ThrowBedbug(p)
			return true
		}
	}
	return false
}

// HandleItemUseOnBlock spawns dream defenders and keeps fire charges from igniting blocks.
func (h *Handler) HandleItemUseOnBlock(ctx *player.Context, pos cube.Pos, face cube.Face, _ mgl64.Vec3) {
	p := ctx.Val()
	if main, _ := p.HeldItems(); main.CustomName() == "§eDream Defender" {
		if g := h.gameOf(); g != nil && g.Playing(h.pl.Name) {
			ctx.Cancel()
			consumeHeld(p)
			at := pos.Side(face)
			g.SpawnDreamDefender(mgl64.Vec3{float64(at[0]) + 0.5, float64(at[1]), float64(at[2]) + 0.5}, h.pl.Name)
		}
		return
	}
	h.useWeapon(ctx)
}

// HandleChat routes chat for players inside an arena and cancels the global broadcast.
func (h *Handler) HandleChat(ctx *player.Context, message *string) {
	if g := h.gameOf(); g != nil {
		ctx.Cancel()
		g.Chat(h.pl, *message)
	}
}

// HandleItemConsume applies the shop potions with their own durations and the magic milk effect.
func (h *Handler) HandleItemConsume(ctx *player.Context, st item.Stack) {
	g := h.gameOf()
	if g == nil || !g.Playing(h.pl.Name) {
		return
	}
	p := ctx.Val()
	type fx struct {
		kind  string
		level int
		secs  int
	}
	var e *fx
	switch st.CustomName() {
	case "§eSpeed Potion II (45 seconds)":
		e = &fx{"speed", 2, 45}
	case "§eJump Potion III (45 seconds)":
		e = &fx{"jump", 4, 45}
	case "§eInvisibility Potion (30 seconds)":
		e = &fx{"invisibility", 2, 30}
	}
	if e != nil {
		ctx.Cancel()
		consumeHeld(p)
		if ef, ok := plat.NewEffect(e.kind, e.level, time.Duration(e.secs)*time.Second, true); ok {
			p.AddEffect(ef)
		}
		return
	}
	if _, ok := st.Item().(item.Bucket); ok && st.CustomName() == "§eMagic Milk" {
		ctx.Cancel()
		consumeHeld(p)
		g.DrinkMilk(h.pl.Name)
		p.Message("§eTrap effected in 30 seconds!")
	}
}

// HandleItemUse handles the lobby team selector, the return bed and the spectator compass, after
// checking for weapons.
func (h *Handler) HandleItemUse(ctx *player.Context) {
	if h.useWeapon(ctx) {
		return
	}
	p := ctx.Val()
	main, _ := p.HeldItems()
	switch main.CustomName() {
	case "Select Team":
		game.P.Mu.Lock()
		g, ok := game.P.InGame[h.pl.Name]
		game.P.Mu.Unlock()
		if ok && g.PhaseNow() == game.PhaseLobby {
			p.SendForm(newTeamForm(g, h.pl))
		}
	case "§eSpectator":
		if g := h.gameOf(); g != nil {
			g.OpenSpectatorMenu(p)
		}
	case "§aReturn to lobby":
		game.P.Mu.Lock()
		g, ok := game.P.InGame[h.pl.Name]
		game.P.Mu.Unlock()
		if ok && g.PhaseNow() == game.PhaseLobby {
			go g.Quit(h.pl)
		}
	}
}

// teamMenu is the team selector form.
type teamMenu struct {
	g     *game.Game
	pl    *plat.Player
	byBtn map[string]string
}

// newTeamForm builds the team selector for a lobby.
func newTeamForm(g *game.Game, pl *plat.Player) form.Menu {
	tm := &teamMenu{g: g, pl: pl, byBtn: map[string]string{}}
	var buttons []form.Button
	for _, t := range g.TeamEntries() {
		text := t.Color + util.Ucfirst(t.Name) + " Team\n§aPlayers: §e" + strconv.Itoa(t.Count)
		tm.byBtn[text] = t.Name
		buttons = append(buttons, form.NewButton(text, ""))
	}
	return form.NewMenu(tm, "Select Team").WithButtons(buttons...)
}

// Submit implements form.MenuSubmittable.
func (t *teamMenu) Submit(_ form.Submitter, pressed form.Button, _ *world.Tx) {
	team, ok := t.byBtn[pressed.Text]
	if !ok {
		return
	}
	go t.g.SelectTeam(t.pl, team)
}
