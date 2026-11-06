package game

import (
	"strconv"
	"time"

	"github.com/df-mc/dragonfly/server/player"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Trap identifiers.
const (
	trapIts     = "itsTrap"
	trapCounter = "counterTrap"
	trapAlarm   = "alarmTrap"
	trapMiner   = "minerTrap"
)

// trapNames maps trap identifiers to display names.
var trapNames = map[string]string{
	trapIts: "It's a trap!", trapCounter: "Counter-Offensive Trap", trapAlarm: "Alarm Trap", trapMiner: "Miner Fatigue Trap",
}

// forgeNames maps the next forge level to its name.
var forgeNames = map[int]string{1: "Iron Forge", 2: "Golden Forge", 3: "Emerald Forge", 4: "Molten Forge"}

// costUpgrade returns the diamond price of the next tier of kind.
func (g *Game) costUpgrade(team, kind string) int { return 2 * g.UtilityArena[team][kind] }

// tierUpgrade returns the tier number shown for the next purchase of kind.
func (g *Game) tierUpgrade(team, kind string) int {
	d := g.UtilityArena[team][kind]
	switch kind {
	case "protection", "generator":
		if d == 5 {
			return 4
		}
	case "haste":
		if d == 3 {
			return 2
		}
	}
	return d
}

// trapCost returns the diamond price of the next trap (1, 2 then 4).
func (g *Game) trapCost(team string) int {
	switch len(g.Traps[team]) {
	case 0:
		return 1
	case 1:
		return 2
	}
	return 4
}

// upgradeEntry is one button of the upgrade menu.
type upgradeEntry struct {
	Text string
	Do   func(p *player.Player)
}

// teamMessage sends a message to every member of the team.
func (g *Game) teamMessage(team, msg string) {
	for _, pt := range g.Players {
		if g.getTeam(pt) == team {
			pt.Msg(msg)
		}
	}
}

// teamSound plays a sound for every member of the team.
func (g *Game) teamSound(team, snd string) {
	for _, pt := range g.Players {
		if g.getTeam(pt) == team {
			pt.Sound(snd)
		}
	}
}

// upgradeTeamGens raises the forge level of the team generators.
func (g *Game) upgradeTeamGens(team string) {
	g.UtilityArena[team]["generator"]++
	tg := g.Data.TeamGenerator[team]
	for _, ge := range g.Gens {
		key := ge.Kind
		// Runtime team emerald generators share the arena's "emerald" position key.
		if key == "emeraldTeam" {
			key = "emerald"
		}
		if ge.Kind != "iron" && ge.Kind != "gold" && ge.Kind != "emeraldTeam" {
			continue
		}
		s, ok := tg[key]
		if ok && ge.Pos.Distance(util.MustVec(s).Add(0.5, 0, 0.5)) < 2 {
			ge.Level++
		}
	}
}

// buildUpgradeMenu builds the menu body and entries. The caller holds the plugin lock.
func (g *Game) buildUpgradeMenu(pl *plat.Player, p *player.Player) (body string, entries []upgradeEntry) {
	team := g.getTeam(pl)
	ua := g.UtilityArena[team]
	display := p.Name()
	diamonds := func(n int) string { return "§b" + strconv.Itoa(n) + " Diamond" }

	buy := func(cost int, owned bool, apply func(), msg string) func(p *player.Player) {
		return func(p *player.Player) {
			g.P.Mu.Lock()
			defer g.P.Mu.Unlock()
			if g.Phase != PhaseGame || g.getTeam(pl) != team {
				return
			}
			if owned {
				return
			}
			if !g.pay(p, "Diamond", cost) {
				return
			}
			apply()
			g.teamSound(team, "random.levelup")
			g.teamMessage(team, "§a"+display+" purchased §6"+msg)
		}
	}
	state := func(name string, cost int, owned bool) string {
		if owned {
			return "§c" + name + "\n§aUnlocked"
		}
		return "§c" + name + "\n" + diamonds(cost)
	}

	sharpOwned := ua["sharpness"] >= 2
	entries = append(entries, upgradeEntry{state("Sharpened Swords", 4, sharpOwned),
		buy(4, sharpOwned, func() {
			ua["sharpness"]++
			for _, pt := range g.Players {
				if g.getTeam(pt) == team {
					pt.SharpenSwords()
				}
			}
		}, "Sharpened Swords")})

	protMax := ua["protection"] >= 5
	protName := "Reinforced Armor " + roman(g.tierUpgrade(team, "protection"))
	entries = append(entries, upgradeEntry{state(protName, g.costUpgrade(team, "protection"), protMax),
		buy(g.costUpgrade(team, "protection"), protMax, func() {
			ua["protection"]++
			for _, pt := range g.Players {
				if g.getTeam(pt) == team {
					g.setArmor(pt)
				}
			}
		}, "Reinforced Armor "+roman(ua["protection"]))})

	hasteMax := ua["haste"] >= 3
	hasteName := "Maniac Miner " + roman(g.tierUpgrade(team, "haste"))
	entries = append(entries, upgradeEntry{state(hasteName, g.costUpgrade(team, "haste"), hasteMax),
		buy(g.costUpgrade(team, "haste"), hasteMax, func() { ua["haste"]++ }, "Maniac Miner "+roman(ua["haste"]))})

	genMax := ua["generator"] >= 5
	next := ua["generator"]
	if next > 4 {
		next = 4
	}
	forge := forgeNames[next]
	entries = append(entries, upgradeEntry{state(forge, g.costUpgrade(team, "generator"), genMax),
		buy(g.costUpgrade(team, "generator"), genMax, func() { g.upgradeTeamGens(team) }, forge)})

	healOwned := ua["health"] >= 2
	entries = append(entries, upgradeEntry{state("Heal Pool", 4, healOwned),
		buy(4, healOwned, func() { ua["health"]++ }, "Heal Pool")})

	trapFull := len(g.Traps[team]) >= 3
	cost := g.trapCost(team)
	for _, kind := range []string{trapIts, trapCounter, trapAlarm, trapMiner} {
		kind := kind
		entries = append(entries, upgradeEntry{state(trapNames[kind], cost, trapFull),
			buy(cost, trapFull, func() { g.Traps[team] = append(g.Traps[team], kind) }, trapNames[kind])})
	}

	for i := 0; i < 3; i++ {
		name := "No Trap!"
		if i < len(g.Traps[team]) {
			name = trapNames[g.Traps[team][i]]
		}
		body += "§cTrap #" + strconv.Itoa(i+1) + ": §7" + name + "\n"
	}
	return body, entries
}

// OpenUpgrade shows the team upgrade and trap menu. Call it from inside the player transaction.
func (g *Game) OpenUpgrade(p *player.Player) {
	g.P.Mu.Lock()
	pl := g.P.Online[p.Name()]
	_, spec := g.Spectators[p.Name()]
	if pl == nil || g.Phase != PhaseGame || !g.inGame(pl) || spec || g.UtilityArena[g.getTeam(pl)] == nil {
		g.P.Mu.Unlock()
		return
	}
	body, entries := g.buildUpgradeMenu(pl, p)
	g.P.Mu.Unlock()

	texts := make([]string, 0, len(entries))
	for _, e := range entries {
		texts = append(texts, e.Text)
	}
	p.SendForm(newMenu("Upgrades & Traps", body, texts, func(p *player.Player, i int) {
		if i < len(entries) {
			entries[i].Do(p)
		}
		g.OpenUpgrade(p)
	}))
}

// OnMove handles player movement: it returns lobby players who fall into the void and triggers
// enemy traps. The caller holds the plugin lock.
func (g *Game) OnMove(pl *plat.Player, pos util.Vec) {
	switch g.Phase {
	case PhaseLobby:
		wp := util.MustVec(arenaStr(g))
		if pos.Y < wp.Y-3 {
			pl.TeleportTo(g.World, mgl64.Vec3{wp.X, wp.Y, wp.Z})
		}
		return
	case PhaseRestart:
		return
	}
	if _, milk := g.IfMilk[pl.Name]; milk {
		return
	}
	if _, sp := g.Spectators[pl.Name]; sp {
		return
	}
	own := g.getTeam(pl)
	for _, team := range g.Data.TeamName {
		bed, ok := g.Data.TeamBed[team]
		if !ok || team == own || len(g.Traps[team]) == 0 {
			continue
		}
		if pos.Distance(util.MustVec(bed)) >= 15 {
			continue
		}
		g.fireTrap(team, pl)
	}
}

// arenaStr returns the waiting spawn string of the arena.
func arenaStr(g *Game) string {
	if g.Data.WaitingSpawn == nil {
		return "0:0:0"
	}
	return *g.Data.WaitingSpawn
}

// fireTrap triggers the first queued trap of team against the intruder. Effect durations are in
// seconds.
func (g *Game) fireTrap(team string, intruder *plat.Player) {
	kind := g.Traps[team][0]
	g.Traps[team] = g.Traps[team][1:]
	title := util.Msg("messages.trap-triggered-title")
	defenders := func(sub string, extra func(*plat.Player)) {
		for _, p := range g.Players {
			if g.getTeam(p) == team {
				p.Title20(title, sub)
				p.Sound("mob.endermen.stare")
				if extra != nil {
					extra(p)
				}
			}
		}
	}
	switch kind {
	case trapIts:
		intruder.AddEffect("blindness", 1, 5*time.Second, true)
		intruder.AddEffect("slowness", 1, 5*time.Second, true)
		defenders(util.Msg("messages.trap-it-a-trap"), nil)
	case trapCounter:
		defenders(util.Msg("messages.trap-counter-offensive"), func(p *plat.Player) {
			p.AddEffect("speed", 1, 15*time.Second, true)
		})
	case trapAlarm:
		intruder.Title20(util.Msg("messages.trap-alarm-title"),
			util.Msg("messages.trap-alarm-subtitle", "teamcolor", g.Data.TeamColor[team], "team", team))
		intruder.Sound("mob.endermen.stare")
		intruder.RemoveEffect("invisibility")
		defenders(util.Msg("messages.trap-alarm"), nil)
	case trapMiner:
		intruder.AddEffect("miningfatigue", 1, 15*time.Second, true)
		defenders(util.Msg("messages.trap-miner-fatigue"), nil)
	}
}
